# <img width="10000" height="660" alt="simple-kvm-banner" src="https://github.com/user-attachments/assets/ff01e9cf-1cf2-4810-99db-5ca9d00b2482" />


Simple KVM is a very basic IP KVM for when PiKVM is overkill. Simple KVM has a simple but responsive UI that works well on both desktop and mobile. The following features are implemented:

- Video ouptut
- Keyboard input, both physical keyboards and mobile soft keyboards
- Responsive UI that works on both desktop and mobile
- Screenshot button
- Simple authentication and session management for one user
- PWA support

<img width="2628" height="1097" alt="kvm-image" src="https://github.com/user-attachments/assets/d9326b62-eb2c-492c-9596-3a0d7715293f" />

## Requirements
- A SBC with at least 1x USB A port and 1x USB-OTG port, for example Raspberry Pi 4.
- [µStreamer](https://github.com/pikvm/ustreamer), a lightweight and fast MJPEG-HTTP streamer
- A cheap USB/HDMI capture card, [for example this one](https://www.amazon.com/dp/B0C2MDTY8P)
- If your SBC has one USB C port that combines power and USB-OTG you also need a splitter, [such as this one](https://www.aliexpress.com/item/1005007723101153.html)

## Setup 
### Video capture
Plug the USB capture card into a USB A port on the SBC and connect an HDMI cable between it and the server.  

### Keyboard HID Input 
If you are using a power/data splitter as descibed above now is the time to plug it in. Connect a USB C to C cable between the SBC and the RPi4 port on the splitter. Then plug the powersupply into the power port, and finally use a USB A to C cable to connect the server and with the splitters data port.

Add this to `/boot/config.txt` or `/boot/firmware/config.txt` (whichever exists) and reboot:
```
dtoverlay=dwc2,dr_mode=peripheral
```

Load the USB gadget kernel module at boot, then reboot:
```bash
echo libcomposite | sudo tee /etc/modules-load.d/libcomposite.conf
sudo reboot
```

Check that a UDC exists: `ls /sys/class/udc` (shows fe980000.usb).

The Simple KVM container creates the USB keyboard gadget itself and keeps an eye on it, so nothing else needs to run on the host. For that, the container mounts `/sys/kernel/config` and is allowed to open the `hidg` device (see `device_cgroup_rules` in the compose file). On first start after a boot the container creates the gadget and then restarts itself once to get access to the new device. This is expected.

### Simple KVM and µStreamer
Clone the repo and prepare the docker compose file:
```bash
git clone https://github.com/nikerl/simple-kvm.git
cd simple-kvm
cp conf/docker-compose_template.yml ./docker-compose.yml
```

Enter the username and password hash for the admin account in the docker compose file. The password hash can be generated [here, for example](https://bcrypt-generator.com/). Note the dollar signs in the password hash need to be doubled to work: `$ -> $$`

Finally build and run the compose stack:
```bash
docker compose up -d
```

## Debugging
The coloured dot next to the title shows the keyboard status (hover for details): green when connected, yellow when the target is asleep or the KVM is recovering, red when the target isn't connected or the gadget couldn't be set up.

The app checks the gadget every 2 seconds and repairs it automatically: it rebuilds it if it's missing or broken, and resets the USB connection if the target stops accepting keystrokes. Check `docker logs simple-kvm` for lines starting with `gadget:` to see what it did.

If the logs say it can't open `/dev/hidg0` with "operation not permitted", the `hidg` major number differs from the one in the compose file. Check it with `grep hidg /proc/devices` and update `device_cgroup_rules`.

```bash
# Check the current USB state, should print "configured"
cat /sys/class/udc/fe980000.usb/state
```

## Security
NEVER expose this IP KVM to the internet, always use it behind a VPN. The included auth is there to prevent someone on your network from stubling onto the KVM and messing with things. It is not designed or tested for being exposed to the internet. 
