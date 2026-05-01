@echo off
title USB AI Studio - Launcher

:: 1. Memastikan working directory berada tepat di lokasi USB dicolokkan
cd /d "%~dp0"

:: 2. Memberikan UI CLI yang rapi kepada pengguna
echo ===================================================
echo             MEMULAI USB AI STUDIO
echo ===================================================
echo.
echo Menghidupkan AI Engine lokal...
echo Mohon tunggu sekitar 3-5 detik, browser akan otomatis terbuka.
echo.

:: 3. Menjalankan middleware Go
:: Tanda "" kosong di awal adalah trik CMD Windows agar path yang memiliki spasi tidak error
start "" "ai-engine-win.exe"

:: 4. Menutup jendela terminal hitam (CMD) ini agar layar pengguna tetap bersih
exit