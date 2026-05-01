#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$DIR"

echo "Memulai USB AI Studio untuk Linux..."
chmod +x ai-engine-linux bin/llama-server-linux
./ai-engine-linux