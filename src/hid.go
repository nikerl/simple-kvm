package main

import (
	"os"
	"strings"
)

var hidState = [8]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

func applyModifier(modifier byte, event string) {
	switch event {
	case "down":
		hidState[0] += modifier
	case "up":
		hidState[0] -= modifier
	}
}

func applyKey(key byte, event string) {
	for i := 2; i < 8; i++ {
		switch event {
		case "down":
			if hidState[i] == 0 {
				hidState[i] = key
				return
			}
		case "up":
			if hidState[i] == key {
				hidState[i] = 0
				return
			}
		}
	} 
}

func sendKeystrokesToServer(keystroke string, hidg *os.File) {
	var split = strings.Split(keystroke, ":")

	// Release all keys
	if len(split) == 1 && split[0] == "release_all" {
		// Write the zeros to hidg to release all keys
		hidg.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return
	}

	var kbtype = split[0]
	var keyName = split[1]
	
	switch kbtype {
	case "hwkb":
		var event = split[2]

		// if keyName is found in the keyMap its a normal key
		if keyCode, ok := keyMap[keyName]; ok {
			applyKey(keyCode, event)

		} else {
			// if not check if its a modifier
			if keyCode, ok := modifierMap[keyName]; ok {
				applyModifier(keyCode, event)

			} else {
				// if not return, bad input
				return
			}
		}

	case "swkb":
		// TODO: Mobile support not implemented yet
	}

	// Write the state to the hidg
	hidg.Write(hidState[:])
}


// KeyboardEvent.code -> HID code (usage page 0x07).
// Regular keys: usage ID for one of the six key slots (bytes 2-7).
var keyMap = map[string]byte{
	// Function row
	"Escape": 0x29,
	"F1":  0x3a, "F2":  0x3b, "F3":  0x3c, "F4":  0x3d,
	"F5":  0x3e, "F6":  0x3f, "F7":  0x40, "F8":  0x41,
	"F9":  0x42, "F10": 0x43, "F11": 0x44, "F12": 0x45,
	"PrintScreen": 0x46, "ScrollLock": 0x47, "Pause": 0x48,

	// Number row
	"Backquote": 0x35,
	"Digit1": 0x1e, "Digit2": 0x1f, "Digit3": 0x20, "Digit4": 0x21,
	"Digit5": 0x22, "Digit6": 0x23, "Digit7": 0x24, "Digit8": 0x25,
	"Digit9": 0x26, "Digit0": 0x27,
	"Minus": 0x2d, "Equal": 0x2e, "Backspace": 0x2a,

	// QWERTY row
	"Tab": 0x2b,
	"KeyQ": 0x14, "KeyW": 0x1a, "KeyE": 0x08, "KeyR": 0x15,
	"KeyT": 0x17, "KeyY": 0x1c, "KeyU": 0x18, "KeyI": 0x0c,
	"KeyO": 0x12, "KeyP": 0x13,
	"BracketLeft": 0x2f, "BracketRight": 0x30, "Backslash": 0x31,

	// Home row
	"CapsLock": 0x39,
	"KeyA": 0x04, "KeyS": 0x16, "KeyD": 0x07, "KeyF": 0x09,
	"KeyG": 0x0a, "KeyH": 0x0b, "KeyJ": 0x0d, "KeyK": 0x0e,
	"KeyL": 0x0f,
	"Semicolon": 0x33, "Quote": 0x34, "Enter": 0x28,

	// Bottom row
	"KeyZ": 0x1d, "KeyX": 0x1b, "KeyC": 0x06, "KeyV": 0x19,
	"KeyB": 0x05, "KeyN": 0x11, "KeyM": 0x10,
	"Comma": 0x36, "Period": 0x37, "Slash": 0x38,
	"IntlBackslash": 0x64, // the <> key next to right shift on ISO
	"Space": 0x2c,

	// Navigation block
	"Insert": 0x49, "Home": 0x4a, "PageUp": 0x4b,
	"Delete": 0x4c, "End":  0x4d, "PageDown": 0x4e,

	// Arrows
	"ArrowUp": 0x52, "ArrowLeft": 0x50, "ArrowDown": 0x51, "ArrowRight": 0x4f,

	// Numpad
	"NumLock": 0x53, "NumpadDivide": 0x54, "NumpadMultiply": 0x55,
	"NumpadSubtract": 0x56, "NumpadAdd": 0x57, "NumpadEnter": 0x58,
	"Numpad1": 0x59, "Numpad2": 0x5a, "Numpad3": 0x5b,
	"Numpad4": 0x5c, "Numpad5": 0x5d, "Numpad6": 0x5e,
	"Numpad7": 0x5f, "Numpad8": 0x60, "Numpad9": 0x61,
	"Numpad0": 0x62, "NumpadDecimal": 0x63,

	// Exclusive to ISO layout
	"IntlRo": 0x32,       // the '* key left of Enter
	"ContextMenu": 0x65,  // menu key
}

// The eight modifier, their value is the bit they set/clear in byte 0, 
// they never go in a slot.
var modifierMap = map[string]byte{
	"ControlLeft":  0x01, "ControlRight":  0x10,
	"ShiftLeft":    0x02, "ShiftRight":    0x20,
	"AltLeft":      0x04, "AltRight":      0x40,
	"MetaLeft":     0x08, "MetaRight":     0x80,
}
