
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
    if (!e.code) return;                                   // Android "Unidentified" noise
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
    for (let n = prev.length - i; n > 0; n--) send('swkb:Backspace');
    const added = cur.slice(i);
    if (added) send('swkb:' + added);
    prev = cur;
    if (!composing) reset();
}

kbin.addEventListener('input', onInput);
kbin.addEventListener('compositionstart', () => { composing = true; });
kbin.addEventListener('compositionend', () => { composing = false; reset(); });


// Show keyboard button
document.getElementById('kbtn').addEventListener('click', () => {
    reset();
    kbin.focus();
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