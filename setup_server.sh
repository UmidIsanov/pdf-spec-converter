#!/usr/bin/env bash
# Установка конвертера на сервере (только Python, без Node, без Docker, без sudo).
# Запуск из папки проекта: bash setup_server.sh
set -e
cd "$(dirname "$0")"

echo "== 1. Python =="
command -v python3 >/dev/null || { echo "❌ python3 не найден"; exit 1; }
python3 --version

echo "== 2. Виртуальное окружение =="
if [ ! -d venv ]; then
    if python3 -m venv venv 2>/tmp/venverr; then
        echo "venv создан (со встроенным pip)"
    else
        echo "⚠️ ensurepip недоступен, создаю venv без pip и ставлю pip вручную..."
        rm -rf venv
        python3 -m venv --without-pip venv
        curl -fsSL https://bootstrap.pypa.io/get-pip.py -o /tmp/get-pip.py
        ./venv/bin/python /tmp/get-pip.py
    fi
fi
./venv/bin/python -m pip install -q --upgrade pip

echo "== 3. Зависимости бэкенда (~1-2 мин) =="
./venv/bin/python -m pip install -q -r backend/requirements.txt
echo "✅ Зависимости установлены"

echo
echo "================ ГОТОВО ================"
echo "Дальше 2 шага:"
echo "1) Ключ Gemini (тот, где $10):"
echo "   echo 'GEMINI_API_KEY=ВСТАВЬ_КЛЮЧ' > ~/converter/.env"
echo "2) Запуск:"
echo "   bash ~/converter/start.sh"
echo "========================================"
