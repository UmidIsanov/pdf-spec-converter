#!/usr/bin/env bash
# Запуск конвертера без ручной активации venv.
# Просто выполните: ./run.sh
#
# Скрипт сам находит Python из виртуального окружения и запускает convert.py,
# независимо от того, из какой папки вы его вызываете.

set -e

# Переходим в папку, где лежит этот скрипт (чтобы пути к venv и input_pdfs были верными)
cd "$(dirname "$0")"

# Если venv ещё не создан — создаём и ставим зависимости
if [ ! -d "venv" ]; then
    echo "📦 Виртуальное окружение не найдено. Создаю venv и устанавливаю зависимости..."
    python3 -m venv venv
    ./venv/bin/pip install --upgrade pip
    ./venv/bin/pip install pandas pypdf python-dotenv google-genai openpyxl
fi

# Запускаем скрипт напрямую Python-ом из venv — активация не нужна
exec ./venv/bin/python convert.py "$@"
