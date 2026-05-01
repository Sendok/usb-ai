#!/bin/bash
# Pindah ke direktori USB tempat script ini berada
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$DIR"

echo "Memulai USB AI Studio untuk macOS..."
# Wajib: Berikan izin eksekusi ke binary Go dan llama
chmod +x ai-engine-mac bin/llama-server-mac

# Jalankan middleware Go
./ai-engine-mac