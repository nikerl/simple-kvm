
const kbin = document.getElementById('kbin');

const ws = new WebSocket(
    `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/keystroke`
);
ws.onopen = () => {
    console.log('WebSocket connected');
};
ws.onerror = (error) => {
    console.error('WebSocket error:', error);
};
ws.onclose = () => {
    console.log('WebSocket disconnected');
    document.getElementById('overlay').style.display = 'flex';
};

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
