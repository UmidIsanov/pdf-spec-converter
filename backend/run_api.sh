#!/usr/bin/env bash
# Запуск API-бэкенда без ручной активации venv.
# Использование: ./backend/run_api.sh
set -e

# Корень проекта = папка на уровень выше этого скрипта
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Создаём venv и ставим зависимости при первом запуске
if [ ! -d "venv" ]; then
    echo "📦 Создаю виртуальное окружение..."
    python3 -m venv venv
    ./venv/bin/pip install --upgrade pip
fi

# Доустанавливаем недостающие пакеты бэкенда (быстро, если уже стоят)
./venv/bin/pip install -q -r backend/requirements.txt

echo "🚀 API запущен на http://localhost:8000  (docs: http://localhost:8000/docs)"
exec ./venv/bin/python -m uvicorn backend.main:app --reload --port 8000
