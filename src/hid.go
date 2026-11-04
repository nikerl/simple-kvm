package main

import (
	"os"
	"strings"
)

var hidState = [8]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

func applyModifier(modifier byte, event string) {
	switch event {
	case "down":
		hidState[0] |= modifier
	case "up":
		hidState[0] &^= modifier
	}
}

func applyKey(key byte, event string) {
	for i := 2; i < 8; i++ {
		switch event {
		case "down":
			if hidState[i] == key {
				return
			}
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
	// Release all keys
	if keystroke == "release_all" {
		// Write the zeros to hidg to release all keys
		hidState = [8]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		hidg.Write(hidState[:])
		return
	}

	// Hardware keyboard: "hwkb:key:event"
	if strings.HasPrefix(keystroke, "hwkb:") {
		var split = strings.Split(keystroke, ":")
		if len(split) < 3 {
			return
		}

		var keyName = split[1]
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
	}

	// Software keyboard: "swkb:text:<chars>" or "swkb:key:<KeyboardEvent.code>".
	// The payload may itself contain ':' so it is split separately.
	if strings.HasPrefix(keystroke, "swkb:") {
		parts := strings.SplitN(keystroke, ":", 3)
		if len(parts) == 3 {
			switch parts[1] {
			case "text":
				for _, r := range parts[2] {
					typeRune(r, hidg)
				}
			case "key":
				tapKey(parts[2], 0, hidg)
			}
		}
		return
	}

	// Write the state to the hidg
	hidg.Write(hidState[:])
}


// Layout of the machine being controlled. HID sends physical key positions, so
// this must match the layout configured on the target: "sv" or "us".
const keyboardLayout = "sv"

// Press and release a key by KeyboardEvent.code with extra modifiers (bits of
// byte 0) held. Modifiers already held (e.g. a latched Ctrl) are left untouched
// and everything we added is released afterwards.
func tapKey(code string, mods byte, hidg *os.File) {
	keyCode, ok := keyMap[code]
	if !ok {
		return
	}
	prevMods := hidState[0]

	// Modifiers go out in their own report first (and are released last) so
	// hosts never see the key arrive before the modifier.
	if hidState[0]|mods != hidState[0] {
		hidState[0] |= mods
		hidg.Write(hidState[:])
	}
	applyKey(keyCode, "down")
	hidg.Write(hidState[:])

	applyKey(keyCode, "up")
	hidg.Write(hidState[:])
	if hidState[0] != prevMods {
		hidState[0] = prevMods
		hidg.Write(hidState[:])
	}
}

// Type a single character using the configured layout. Unmappable characters
// are ignored.
func typeRune(r rune, hidg *os.File) {
	switch {
	case r >= 'a' && r <= 'z':
		tapKey("Key"+string(r-32), 0, hidg)
		return
	case r >= 'A' && r <= 'Z':
		tapKey("Key"+string(r), modifierMap["ShiftLeft"], hidg)
		return
	case r >= '0' && r <= '9':
		tapKey("Digit"+string(r), 0, hidg)
		return
	}
	k, ok := layouts[keyboardLayout][r]
	if !ok {
		return
	}
	var mods byte
	if k.shift {
		mods |= modifierMap["ShiftLeft"]
	}
	if k.altgr {
		mods |= modifierMap["AltRight"]
	}
	tapKey(k.code, mods, hidg)
	if k.dead {
		// Dead key: a following space makes the accent appear on its own
		tapKey("Space", 0, hidg)
	}
}

type runeKey struct {
	code  string
	shift bool
	altgr bool
	dead  bool // dead key, needs a space after it to produce the character
}

// Non-alphanumeric characters per layout (letters and digits are handled in
// typeRune, they sit on the same keys in both).
var layouts = map[string]map[rune]runeKey{
	"us": {
		' ': {code: "Space"}, '\n': {code: "Enter"}, '\t': {code: "Tab"},
		'`': {code: "Backquote"}, '~': {code: "Backquote", shift: true},
		'-': {code: "Minus"}, '_': {code: "Minus", shift: true},
		'=': {code: "Equal"}, '+': {code: "Equal", shift: true},
		'[': {code: "BracketLeft"}, '{': {code: "BracketLeft", shift: true},
		']': {code: "BracketRight"}, '}': {code: "BracketRight", shift: true},
		'\\': {code: "Backslash"}, '|': {code: "Backslash", shift: true},
		';': {code: "Semicolon"}, ':': {code: "Semicolon", shift: true},
		'\'': {code: "Quote"}, '"': {code: "Quote", shift: true},
		',': {code: "Comma"}, '<': {code: "Comma", shift: true},
		'.': {code: "Period"}, '>': {code: "Period", shift: true},
		'/': {code: "Slash"}, '?': {code: "Slash", shift: true},
		'!': {code: "Digit1", shift: true}, '@': {code: "Digit2", shift: true},
		'#': {code: "Digit3", shift: true}, '$': {code: "Digit4", shift: true},
		'%': {code: "Digit5", shift: true}, '^': {code: "Digit6", shift: true},
		'&': {code: "Digit7", shift: true}, '*': {code: "Digit8", shift: true},
		'(': {code: "Digit9", shift: true}, ')': {code: "Digit0", shift: true},
	},

	// Swedish ISO. AltGr is the right Alt key.
	"sv": {
		' ': {code: "Space"}, '\n': {code: "Enter"}, '\t': {code: "Tab"},
		'§': {code: "Backquote"}, '½': {code: "Backquote", shift: true},

		'!': {code: "Digit1", shift: true},
		'"': {code: "Digit2", shift: true}, '@': {code: "Digit2", altgr: true},
		'#': {code: "Digit3", shift: true}, '£': {code: "Digit3", altgr: true},
		'¤': {code: "Digit4", shift: true}, '$': {code: "Digit4", altgr: true},
		'%': {code: "Digit5", shift: true}, '€': {code: "Digit5", altgr: true},
		'&': {code: "Digit6", shift: true},
		'/': {code: "Digit7", shift: true}, '{': {code: "Digit7", altgr: true},
		'(': {code: "Digit8", shift: true}, '[': {code: "Digit8", altgr: true},
		')': {code: "Digit9", shift: true}, ']': {code: "Digit9", altgr: true},
		'=': {code: "Digit0", shift: true}, '}': {code: "Digit0", altgr: true},

		'+': {code: "Minus"}, '?': {code: "Minus", shift: true},
		'\\': {code: "Minus", altgr: true},
		'´': {code: "Equal", dead: true}, '`': {code: "Equal", shift: true, dead: true},

		'å': {code: "BracketLeft"}, 'Å': {code: "BracketLeft", shift: true},
		'¨': {code: "BracketRight", dead: true},
		'^': {code: "BracketRight", shift: true, dead: true},
		'~': {code: "BracketRight", altgr: true, dead: true},

		'ö': {code: "Semicolon"}, 'Ö': {code: "Semicolon", shift: true},
		'ä': {code: "Quote"}, 'Ä': {code: "Quote", shift: true},
		'\'': {code: "Backslash"}, '*': {code: "Backslash", shift: true},

		'<': {code: "IntlBackslash"}, '>': {code: "IntlBackslash", shift: true},
		'|': {code: "IntlBackslash", altgr: true},

		',': {code: "Comma"}, ';': {code: "Comma", shift: true},
		'.': {code: "Period"}, ':': {code: "Period", shift: true},
		'-': {code: "Slash"}, '_': {code: "Slash", shift: true},
	},
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
