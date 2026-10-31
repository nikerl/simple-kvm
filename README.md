# Simple KVM
Simple KVM is a very basic IP KVM for when PiKVM is overkill. 

## Requirements
- A SBC with at least 1x USB A port and 1x USB-OTG port, for example Raspberry Pi 4.
- [µStreamer](https://github.com/pikvm/ustreamer), a lightweight and fast MJPEG-HTTP streamer
- A cheap USB/HDMI capture card, [for example this one](https://www.amazon.com/dp/B0C2MDTY8P)
- If your SBC has one USB C port that combines power and USB-OTG you also need a splitter, [such as this one](https://www.aliexpress.com/item/1005007723101153.html)

## Setup
The following environment variables need to be set or present in a .env file:
```
USTREAMER_URL='<url_to_ustreamer>'
ADMIN_USERNAME='<admin_username>'
ADMIN_PASSWORD_HASH='<bcrypt_hash_of_admin_password>'
```

## Security
NEVER expose this IP KVM to the internet, always use it behind a VPN.
