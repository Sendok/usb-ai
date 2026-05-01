const messagesDiv = document.getElementById('messages');
const userInput = document.getElementById('userInput');
const actionBtn = document.getElementById('actionBtn');

let isGenerating = false;
let abortController = null;

// Konfigurasi Marked.js untuk integrasi dengan Prism.js
marked.setOptions({
    highlight: function(code, lang) {
        if (Prism.languages[lang]) {
            return Prism.highlight(code, Prism.languages[lang], lang);
        } else {
            return code;
        }
    }
});

function toggleAction() {
    if (isGenerating) {
        if (abortController) abortController.abort(); // Stop generasi
    } else {
        askAI(); // Mulai generasi
    }
}

async function askAI() {
    const prompt = userInput.value.trim();
    if (!prompt) return;

    // Tampilkan pesan user
    addMessage('user', prompt);
    userInput.value = '';
    
    // Siapkan UI untuk pesan AI
    const aiMessageContainer = addMessage('assistant', '');
    
    // Ubah tombol jadi "Stop"
    isGenerating = true;
    actionBtn.innerText = '🛑';
    actionBtn.classList.add('stop');

    abortController = new AbortController();

    try {
        let rawMarkdown = "";
        const response = await fetch('/api/chat', {
            method: 'POST',
            body: JSON.stringify({ prompt }),
            headers: { 'Content-Type': 'application/json' },
            signal: abortController.signal
        });

        const reader = response.body.getReader();
        const decoder = new TextDecoder();

        while (true) {
            const { value, done } = await reader.read();
            if (done) break;

            const chunk = decoder.decode(value, { stream: true });
            rawMarkdown += chunk;

            // Render Markdown -> Sanitasi -> Tampilkan
            const rawHtml = marked.parse(rawMarkdown);
            const safeHtml = DOMPurify.sanitize(rawHtml);
            aiMessageContainer.innerHTML = safeHtml;
            
            // Re-run Prism.js untuk syntax highlighting kode baru
            Prism.highlightAllUnder(aiMessageContainer);
            
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }
    } catch (err) {
        if (err.name === 'AbortError') {
            aiMessageContainer.innerHTML += "<br><i>[Generasi dihentikan]</i>";
        } else {
            aiMessageContainer.innerHTML += "<br><i>[Error koneksi ke Engine]</i>";
        }
    } finally {
        // Kembalikan tombol ke kondisi awal
        isGenerating = false;
        actionBtn.innerText = '➤';
        actionBtn.classList.remove('stop');
        abortController = null;
    }
}

function addMessage(role, text) {
    const div = document.createElement('div');
    div.className = `message ${role}`;
    div.innerText = text;
    messagesDiv.appendChild(div);
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
    return div;
}

// Handler Enter key
userInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        if (!isGenerating) askAI();
    }
});