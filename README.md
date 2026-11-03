# Simple KVM
Simple KVM is a very basic IP KVM for when PiKVM is overkill. 

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

Add this to /boot/config.txt or /boot/firmware/config.txt (whichever exists) and reboot:
```
dtoverlay=dwc2,dr_mode=peripheral
```

Check that a UDC exists: `ls /sys/class/udc` (shows fe980000.usb).

Install and run the USB Gadget script
```bash
sudo cp conf/kvm-gadget.sh /usr/local/bin/
sudo chmod +x /usr/local/bin/kvm-gadget.sh
sudo bash -x /usr/local/bin/kvm-gadget.sh
``` 

Create a service that runs on startup to install the gadget
```bash
sudo cp conf/kvm-gadget.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now kvm-gadget.service
```

Test the USB gadget to make sure it works
```bash
printf '\0\0\x0b\0\0\0\0\0' > /dev/hidg0   # press h
printf '\0\0\0\0\0\0\0\0'   > /dev/hidg0   # release
```

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

## Security
NEVER expose this IP KVM to the internet, always use it behind a VPN.
