
const kbin = document.getElementById('kbin');

let ws;
let reconnectTimer = null;
let hasConnected = false;

const connect = () => {
    ws = new WebSocket(
        `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/keystroke`
    );
    ws.onopen = () => {
        console.log('WebSocket connected');
        if (hasConnected) reloadStream();   // reconnect: the stream probably died too
        hasConnected = true;
    };
    ws.onerror = (error) => {
        console.error('WebSocket error:', error);
    };
    ws.onclose = () => {
        console.log('WebSocket disconnected');
        scheduleReconnect(500);
    };
};

// Only show the sign-in overlay if the session is really gone. Otherwise
// (idle timeout on a proxy, network blip, server restart) just reconnect.
const scheduleReconnect = (delay) => {
    clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(async () => {
        try {
            const r = await fetch('/', { redirect: 'manual', cache: 'no-store' });
            if (r.type === 'opaqueredirect' || r.status === 401 || r.status === 303) {
                document.getElementById('overlay').style.display = 'flex';
                return;
            }
            connect();
        } catch (e) {
            scheduleReconnect(Math.min(delay * 2, 5000));   // server unreachable, retry
        }
    }, delay);
};

connect();

// Keep the socket from being closed for inactivity by proxies/NAT
setInterval(() => {
    if (ws.readyState === WebSocket.OPEN) ws.send('ping');
}, 20000);

// Restart the MJPEG stream. Mobile browsers suspend or kill the connection while the
// tab/PWA is in the background, and an <img> never reconnects by itself (goes black).
const screenImg = document.getElementById('screen');
let streamRetry = null;
const reloadStream = () => {
    clearTimeout(streamRetry);
    screenImg.src = '/stream?t=' + Date.now();   // new URL = new connection, old one is aborted
};
screenImg.addEventListener('error', () => {
    clearTimeout(streamRetry);
    streamRetry = setTimeout(reloadStream, 2000);
});
let hiddenAt = 0;
document.addEventListener('visibilitychange', () => {
    if (document.hidden) { hiddenAt = Date.now(); return; }
    if (hiddenAt && Date.now() - hiddenAt > 2000) reloadStream();   // skip quick app switches
});
window.addEventListener('pageshow', (e) => { if (e.persisted) reloadStream(); });

// Reconnect right away when the tab becomes visible again
document.addEventListener('visibilitychange', () => {
    if (!document.hidden && ws.readyState !== WebSocket.OPEN && ws.readyState !== WebSocket.CONNECTING) {
        scheduleReconnect(0);
    }
});

const SENTINEL = ' ';

const send = (m) => {
    if (ws.readyState === WebSocket.OPEN) {
        ws.send(m);
    }
};



// Handler for hardware keyboard
const PASS = /^(Enter|Tab|Escape|Arrow\w+|F\d+|Home|End|PageUp|PageDown|Delete|Insert)$/;

document.addEventListener('keydown', (e) => {
    if (!e.code) {                                         // Android "Unidentified" noise
        if (e.target === kbin && e.key === 'Enter') send('swkb:key:Enter');
        return;
    }
    if (e.target === kbin && !PASS.test(e.code)) return;   // let the diff handle it
    if (!e.repeat) send('hwkb:' + e.code + ':down');
    e.preventDefault();
});
document.addEventListener('keyup', (e) => {
    if (!e.code) return;
    if (e.target === kbin && !PASS.test(e.code)) return;
    send('hwkb:' + e.code + ':up');
    e.preventDefault();
});
window.addEventListener('blur', () => send('release_all'));



// software/virtual keyboard: diff the input's value against its previous value 
let prev = SENTINEL;
let composing = false;

function reset() { kbin.value = SENTINEL; prev = SENTINEL; }

function onInput() {
    const cur = kbin.value;
    let i = 0;
    while (i < prev.length && i < cur.length && prev[i] === cur[i]) i++;
    for (let n = prev.length - i; n > 0; n--) send('swkb:key:Backspace');
    const added = cur.slice(i);
    if (added) send('swkb:text:' + added);
    prev = cur;
    if (!composing) reset();
}

kbin.addEventListener('input', onInput);
kbin.addEventListener('compositionstart', () => { composing = true; });
kbin.addEventListener('compositionend', () => { composing = false; reset(); });


// Show keyboard button
document.getElementById('kbtn').addEventListener('click', () => {
    reset();
    kbin.focus({ preventScroll: true });
});

// Screenshot button
document.getElementById('dlbtn').onclick = async () => {
    const url = '/snapshot'; // same-origin URL of the image
    const res = await fetch(url, { cache: 'no-store' });
    if (!res.ok) return alert('Download failed: ' + res.status);
    const blob = await res.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `screenshot-${new Date().toISOString().replace(/[:.]/g, '-')}.jpg`;
    a.click();
    URL.revokeObjectURL(a.href);
};


// Mobile: size the page to the visual viewport so the stream fills the space above the keyboard
const mobile = window.matchMedia('(max-width: 599px)');
if (window.visualViewport) {
    const fit = () => {
        document.documentElement.style.setProperty('--vvh', window.visualViewport.height + 'px');
        window.scrollTo(0, 0);
    };
    window.visualViewport.addEventListener('resize', fit);
    window.visualViewport.addEventListener('scroll', fit);
    fit();
}

// Mobile: open the keyboard by default. Browsers usually require a user gesture,
// so try immediately and again on the first touch.
if (mobile.matches) {
    const open = () => { reset(); kbin.focus({ preventScroll: true }); };
    open();
    // Browsers only show the keyboard from a real tap, so retry on the first gesture
    const onGesture = () => {
        if (document.activeElement !== kbin) open();
        ['touchend', 'pointerup', 'click'].forEach((t) => document.removeEventListener(t, onGesture, true));
    };
    ['touchend', 'pointerup', 'click'].forEach((t) => document.addEventListener(t, onGesture, true));
}


// Header + main padding + frame border: the fixed (non-stream) part of the page height
const setChrome = () => {
    const bar = document.getElementById('keybar');
    const h = document.querySelector('header').offsetHeight + (bar.offsetParent ? bar.offsetHeight : 0);
    document.documentElement.style.setProperty('--chrome', (h + 16 + 2) + 'px');
};
setChrome();
window.addEventListener('resize', setChrome);

// Stream zoom height: latch the largest "large viewport" height seen for the current
// width, so the keyboard opening/closing can never shrink (zoom out) the stream.
// Resets only when the width changes (rotation).
const lvhProbe = document.createElement('div');
lvhProbe.style.cssText = 'position:fixed;top:0;left:0;width:0;height:100lvh;visibility:hidden;pointer-events:none';
document.body.appendChild(lvhProbe);
let baseW = 0, baseH = 0;
const setLvh = () => {
    const w = window.innerWidth;
    const h = lvhProbe.offsetHeight || window.innerHeight;
    if (w !== baseW) { baseW = w; baseH = h; }
    else if (h > baseH) baseH = h;
    document.documentElement.style.setProperty('--lvh', baseH + 'px');
};
setLvh();
window.addEventListener('resize', setLvh);
if (window.visualViewport) window.visualViewport.addEventListener('resize', setLvh);



// Mobile key bar (Ctrl latch + arrows). Buttons never take focus, so the keyboard stays open.
document.querySelectorAll('#keybar button[data-key]').forEach((btn) => {
    const code = btn.dataset.key;
    const toggle = 'toggle' in btn.dataset;
    btn.addEventListener('pointerdown', (e) => {
        e.preventDefault();
        if (toggle) {
            const on = btn.classList.toggle('active');
            send('hwkb:' + code + ':' + (on ? 'down' : 'up'));
        } else {
            btn.setPointerCapture(e.pointerId);
            send('hwkb:' + code + ':down');
        }
    });
    if (!toggle) {
        const up = () => send('hwkb:' + code + ':up');
        btn.addEventListener('pointerup', up);
        btn.addEventListener('pointercancel', up);
    }
    btn.addEventListener('mousedown', (e) => e.preventDefault());
});

window.addEventListener('blur', () => {
    document.querySelectorAll('#keybar button.active').forEach((b) => b.classList.remove('active'));
});


// Keyboard gadget status indicator
const hidstatus = document.getElementById('hidstatus');
async function pollStatus() {
    try {
        const res = await fetch('/status', { cache: 'no-store' });
        const s = await res.json();
        hidstatus.className = 'status ' + s.level;
        hidstatus.title = s.message;
    } catch {
        hidstatus.className = 'status error';
        hidstatus.title = 'KVM server unreachable';
    }
}
pollStatus();
setInterval(pollStatus, 3000);


// Register the service worker (makes the app installable)
if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch((e) => console.warn('SW registration failed:', e));
}
