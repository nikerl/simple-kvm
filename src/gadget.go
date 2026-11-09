package main

// USB HID gadget lifecycle, owned by the app so nothing has to run on the host
// besides loading libcomposite.
//
// The gadget lives in configfs and outlives the app/container, so setup is
// idempotent: a healthy existing gadget is adopted, a broken one is torn down
// and rebuilt. A watchdog keeps it bound and rebinds the UDC when the target
// stops accepting reports.

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	gadgetRoot  = "/sys/kernel/config/usb_gadget"
	gadgetDir   = gadgetRoot + "/kvm"
	udcClassDir = "/sys/class/udc"
	hidgSysDev  = "/sys/class/hidg/hidg0/dev"
	hidgDevPath = "/dev/hidg0"

	watchInterval  = 2 * time.Second
	writeTimeout   = 100 * time.Millisecond
	maxFailures    = 3                // failed writes while "configured" before rebinding
	stuckTimeout   = 15 * time.Second // time in a half-enumerated state before rebinding
	repairCooldown = 15 * time.Second

	// "not attached" can mean the controller is wedged while the target is
	// actually there, so rebind with backoff until it attaches again.
	detachedRetryMin = 30 * time.Second
	detachedRetryMax = 5 * time.Minute
)

// Boot-protocol keyboard: 1 modifier byte, 1 reserved, 5 LEDs (out), 6 keys.
var reportDesc = []byte{
	0x05, 0x01, 0x09, 0x06, 0xa1, 0x01, 0x05, 0x07, 0x19, 0xe0, 0x29, 0xe7,
	0x15, 0x00, 0x25, 0x01, 0x75, 0x01, 0x95, 0x08, 0x81, 0x02, 0x95, 0x01,
	0x75, 0x08, 0x81, 0x03, 0x95, 0x05, 0x75, 0x01, 0x05, 0x08, 0x19, 0x01,
	0x29, 0x05, 0x91, 0x02, 0x95, 0x01, 0x75, 0x03, 0x91, 0x03, 0x95, 0x06,
	0x75, 0x08, 0x15, 0x00, 0x25, 0x65, 0x05, 0x07, 0x19, 0x00, 0x29, 0x65,
	0x81, 0x00, 0xc0,
}

type Gadget struct {
	mu         sync.Mutex // guards everything below, and serialises writes
	f          *os.File
	udc        string
	state      string // last seen UDC state
	stateSince time.Time
	failures   int
	lastRepair time.Time
	lastErr    string
	retryEvery time.Duration // current backoff for "not attached" rebinds
}

var gadget = &Gadget{}

// Whether the hidg char major existed when we started. The kernel only
// registers it once a gadget HID function exists, and Docker (with the systemd
// cgroup driver) can only apply the device_cgroup_rules entry for a major that
// exists at container start. See reopen().
var hidgMajorAtStart = strings.Contains(readAttr("/proc/devices"), " hidg\n")

// ---- configfs helpers ----

func writeAttr(path, value string) error {
	return os.WriteFile(path, []byte(value), 0644)
}

func readAttr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func firstUDC() (string, error) {
	entries, err := os.ReadDir(udcClassDir)
	if err != nil || len(entries) == 0 {
		return "", errors.New("no UDC found (is dtoverlay=dwc2,dr_mode=peripheral set?)")
	}
	return entries[0].Name(), nil
}

func gadgetHealthy() bool {
	desc, err := os.ReadFile(gadgetDir + "/functions/hid.usb0/report_desc")
	if err != nil || string(desc) != string(reportDesc) {
		return false
	}
	_, err = os.Lstat(gadgetDir + "/configs/c.1/hid.usb0")
	return err == nil
}

func createGadget() error {
	dirs := []string{
		gadgetDir + "/strings/0x409",
		gadgetDir + "/configs/c.1/strings/0x409",
		gadgetDir + "/functions/hid.usb0",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	attrs := [][2]string{
		{"idVendor", "0x1d6b"},
		{"idProduct", "0x0104"},
		{"bcdUSB", "0x0200"},
		{"strings/0x409/serialnumber", "kvm0001"},
		{"strings/0x409/manufacturer", "DIY"},
		{"strings/0x409/product", "Pi KVM"},
		{"configs/c.1/strings/0x409/configuration", "HID"},
		{"functions/hid.usb0/protocol", "1"},
		{"functions/hid.usb0/subclass", "1"},
		{"functions/hid.usb0/report_length", "8"},
		{"functions/hid.usb0/report_desc", string(reportDesc)},
	}
	for _, a := range attrs {
		if err := writeAttr(gadgetDir+"/"+a[0], a[1]); err != nil {
			return fmt.Errorf("%s: %w", a[0], err)
		}
	}
	return os.Symlink(gadgetDir+"/functions/hid.usb0", gadgetDir+"/configs/c.1/hid.usb0")
}

// Remove the gadget. configfs requires this exact order and rmdir (not
// recursive removal); missing pieces are ignored so a half-built gadget can be
// cleaned up too.
func teardownGadget() {
	writeAttr(gadgetDir+"/UDC", "\n")
	for _, p := range []string{
		"configs/c.1/hid.usb0",
		"configs/c.1/strings/0x409",
		"configs/c.1",
		"functions/hid.usb0",
		"strings/0x409",
		"",
	} {
		if err := os.Remove(filepath.Join(gadgetDir, p)); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("gadget: teardown %s: %v", p, err)
		}
	}
}

// ---- device node ----

func mkdev(major, minor uint64) uint64 {
	return (minor & 0xff) | (major&0xfff)<<8 | (minor&^0xff)<<12 | (major&^0xfff)<<32
}

// Make sure /dev/hidg0 matches the kernel's current device number. Inside a
// container /dev is private, so the node has to be created by us; on the host
// udev has already made it and this is a no-op.
func ensureDevNode() (major uint64, err error) {
	majMin := strings.SplitN(readAttr(hidgSysDev), ":", 2)
	if len(majMin) != 2 {
		return 0, errors.New("hidg0 not present in /sys/class/hidg")
	}
	major, err1 := strconv.ParseUint(majMin[0], 10, 32)
	minor, err2 := strconv.ParseUint(majMin[1], 10, 32)
	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("bad hidg0 dev %q", majMin)
	}
	want := mkdev(major, minor)

	var st syscall.Stat_t
	if err := syscall.Stat(hidgDevPath, &st); err == nil {
		if st.Mode&syscall.S_IFMT == syscall.S_IFCHR && uint64(st.Rdev) == want {
			return major, nil
		}
		os.Remove(hidgDevPath)
	}
	if err := syscall.Mknod(hidgDevPath, syscall.S_IFCHR|0600, int(want)); err != nil {
		return major, fmt.Errorf("mknod %s: %w", hidgDevPath, err)
	}
	return major, nil
}

// ---- Gadget ----

// setup creates/adopts the gadget and binds it. Caller holds g.mu.
func (g *Gadget) setup() error {
	// The hidg fd must be closed before the UDC is (re)bound, otherwise the
	// kernel can leave the char device unusable (open gives ENXIO).
	g.closeDev()

	if _, err := os.Stat(gadgetRoot); err != nil {
		return fmt.Errorf("%s missing: load libcomposite on the host and mount /sys/kernel/config into the container", gadgetRoot)
	}
	udc, err := firstUDC()
	if err != nil {
		return err
	}
	g.udc = udc

	if _, err := os.Stat(gadgetDir); err == nil && !gadgetHealthy() {
		log.Println("gadget: existing gadget is incomplete or outdated, rebuilding")
		teardownGadget()
	}
	if _, err := os.Stat(gadgetDir); errors.Is(err, os.ErrNotExist) {
		log.Println("gadget: creating")
		if err := createGadget(); err != nil {
			teardownGadget()
			return fmt.Errorf("create gadget: %w", err)
		}
	}
	if readAttr(gadgetDir+"/UDC") == "" {
		log.Printf("gadget: binding to %s", udc)
		if err := writeAttr(gadgetDir+"/UDC", udc); err != nil {
			return fmt.Errorf("bind UDC: %w", err)
		}
	}
	return g.reopen()
}

func (g *Gadget) closeDev() {
	if g.f != nil {
		g.f.Close()
		g.f = nil
	}
}

// reopen (re)creates the device node and opens it. Caller holds g.mu.
func (g *Gadget) reopen() error {
	g.closeDev()
	major, err := ensureDevNode()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(hidgDevPath, os.O_WRONLY, 0)
	if err != nil {
		if errors.Is(err, os.ErrPermission) && !hidgMajorAtStart {
			// We created the first HID function after the container started,
			// so the cgroup rule couldn't be applied. A restart fixes it, and
			// the gadget persists in the kernel so this only happens once per boot.
			log.Fatalf("gadget: %s was created after startup and the container can't open it yet; exiting so Docker restarts the container", hidgDevPath)
		}
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("open %s: %w (add device_cgroup_rules: 'c %d:* rwm' to the container)", hidgDevPath, err, major)
		}
		return fmt.Errorf("open %s: %w", hidgDevPath, err)
	}
	g.f = f
	g.failures = 0
	return nil
}

// rebind unbinds and rebinds the UDC, which makes the target re-enumerate the
// keyboard. This is what fixes a gadget that silently stopped working.
// Caller holds g.mu.
func (g *Gadget) rebind() error {
	log.Println("gadget: rebinding UDC")
	g.lastRepair = time.Now()
	g.closeDev() // see setup()
	writeAttr(gadgetDir+"/UDC", "\n")
	time.Sleep(500 * time.Millisecond)
	if err := writeAttr(gadgetDir+"/UDC", g.udc); err != nil {
		return fmt.Errorf("rebind UDC: %w", err)
	}
	return g.reopen()
}

// Write sends one HID report. It never blocks for long: if the host isn't
// polling (asleep, unplugged, wedged) the write times out and counts as a
// failure for the watchdog.
func (g *Gadget) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.f == nil {
		if err := g.reopen(); err != nil {
			g.lastErr = err.Error()
			return 0, err
		}
	}
	g.f.SetWriteDeadline(time.Now().Add(writeTimeout))
	n, err := g.f.Write(p)
	if err != nil {
		g.failures++
		g.lastErr = err.Error()
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			g.f.Close()
			g.f = nil
		}
		return n, err
	}
	g.failures = 0
	return n, nil
}

func (g *Gadget) check() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Gadget gone, unbound or never set up
	if g.udc == "" || !gadgetHealthy() || readAttr(gadgetDir+"/UDC") == "" {
		if err := g.setup(); err != nil {
			if err.Error() != g.lastErr {
				log.Println("gadget:", err)
			}
			g.lastErr = err.Error()
			return
		}
		g.lastErr = ""
	}

	state := readAttr(udcClassDir + "/" + g.udc + "/state")
	if state != g.state {
		log.Printf("gadget: UDC state %q -> %q", g.state, state)
		g.state = state
		g.stateSince = time.Now()
		g.failures = 0
	}

	canRepair := time.Since(g.lastRepair) > repairCooldown
	switch state {
	case "configured":
		g.retryEvery = detachedRetryMin
		if g.failures >= maxFailures && canRepair {
			log.Printf("gadget: %d failed writes while configured", g.failures)
			g.repair()
		}
	case "default", "addressed", "powered", "attached", "reconnecting":
		// Host sees us but enumeration never finished
		if time.Since(g.stateSince) > stuckTimeout && canRepair {
			log.Printf("gadget: stuck in %q", state)
			g.repair()
		}
	case "not attached":
		// Usually the cable or target is off, but the dwc2 controller can also
		// get stuck here while the target keeps trying to enumerate. A rebind
		// is harmless when nothing is connected, so retry with backoff.
		if g.retryEvery == 0 {
			g.retryEvery = detachedRetryMin
		}
		if time.Since(g.stateSince) > detachedRetryMin && time.Since(g.lastRepair) > g.retryEvery {
			log.Printf("gadget: not attached, rebinding in case the controller is stuck (next try in %s)", min(g.retryEvery*2, detachedRetryMax))
			g.repair()
			g.retryEvery = min(g.retryEvery*2, detachedRetryMax)
		}
	}
	// "suspended" (target asleep) is left alone: a rebind could wake the
	// target or make it see the keyboard disconnect.

	if g.f == nil {
		err := g.reopen()
		if errors.Is(err, syscall.ENXIO) && canRepair {
			// The gadget is bound but its char device is dead (happens when
			// the UDC was rebound while the device was open). Only a full
			// rebuild brings it back.
			log.Println("gadget: hidg0 unusable, rebuilding gadget")
			g.lastRepair = time.Now()
			g.closeDev()
			teardownGadget()
			err = g.setup()
		}
		if err != nil {
			g.lastErr = err.Error()
		}
	}
}

func (g *Gadget) repair() {
	if err := g.rebind(); err != nil {
		log.Println("gadget:", err)
		g.lastErr = err.Error()
	}
}

func (g *Gadget) Watch() {
	for {
		g.check()
		time.Sleep(watchInterval)
	}
}

type GadgetStatus struct {
	Level   string `json:"level"` // ok, warn, error
	State   string `json:"state"`
	Message string `json:"message"`
}

func (g *Gadget) Status() GadgetStatus {
	g.mu.Lock()
	defer g.mu.Unlock()

	s := GadgetStatus{State: g.state}
	switch {
	case g.f == nil || g.udc == "":
		s.Level, s.Message = "error", "Keyboard unavailable: "+g.lastErr
	case g.state == "configured" && g.failures == 0:
		s.Level, s.Message = "ok", "Keyboard connected"
	case g.state == "configured":
		s.Level, s.Message = "warn", "Target not accepting keystrokes, will reset"
	case g.state == "suspended":
		s.Level, s.Message = "warn", "Target is asleep"
	case g.state == "not attached":
		s.Level, s.Message = "error", "Target not connected"
	default:
		s.Level, s.Message = "warn", "Connecting ("+g.state+")"
	}
	return s
}
