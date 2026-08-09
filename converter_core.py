"""
Общая логика конвертера спецификаций — используется и CLI (convert.py),
и веб-бэкендом (backend/main.py).

Содержит:
  * чтение текста из PDF (из файла или из потока байтов);
  * разбор текста через Gemini в структурированный JSON (с ретраями);
  * сборку и форматирование Excel-книги из уже разобранных данных.
"""

import os
import re
import json
import time
import io

import pypdf
from pypdf import PdfReader
from dotenv import load_dotenv
from google import genai
from google.genai import types
from openpyxl import Workbook
from openpyxl.styles import Font, Alignment, PatternFill
from openpyxl.utils import get_column_letter

# Игнорируем мелкие ошибки разметки внутри PDF из AutoCAD
pypdf.PdfReader.strict = False

load_dotenv()

# Модель Gemini для парсинга (можно переопределить через переменную окружения)
GEMINI_MODEL = os.getenv("GEMINI_MODEL", "gemini-3.6-flash")

# Ограничения ширины колонок Excel
MAX_COLUMN_WIDTH = 60
MIN_COLUMN_WIDTH = 10

# Порядок и подписи колонок итоговой таблицы: (заголовок в Excel, ключ в JSON от ИИ)
ITEM_COLUMNS = [
    ("Раздел", "section"),
    ("Поз.", "pos"),
    ("Наименование и техническая характеристика", "name"),
    ("Тип, марка, обозначение", "type_code"),
    ("Код продукции", "product_code"),
    ("Поставщик", "supplier"),
    ("Ед. изм.", "unit"),
    ("Кол-во", "quantity"),
    ("Масса 1 ед, кг", "weight_kg"),
    ("Примечание", "note"),
]

# Колонки уровня документа (идут перед позиционными)
DOC_COLUMNS = ["Имя файла", "Шифр документа", "Система"]


def all_headers():
    """Полный упорядоченный список заголовков колонок Excel."""
    return DOC_COLUMNS + [header for header, _ in ITEM_COLUMNS]

# Схема JSON-вывода Gemini (Structured Output)
SPEC_SCHEMA = {
    "type": "OBJECT",
    "properties": {
        "doc_number": {"type": "STRING", "description": "Шифр/номер документа из штампа"},
        "system_name": {"type": "STRING", "description": "Наименование системы/раздела"},
        "items": {
            "type": "ARRAY",
            "items": {
                "type": "OBJECT",
                "properties": {
                    "section": {"type": "STRING", "description": "Раздел спецификации, под которым идёт позиция: например 'Оборудование', 'Кабели и провода', 'Изделия и материалы'. Если раздел не указан — пустая строка."},
                    "pos": {"type": "STRING", "description": "Позиция/Марка"},
                    "name": {"type": "STRING", "description": "Наименование и техническая характеристика"},
                    "type_code": {"type": "STRING", "description": "Тип, марка, обозначение документа, опросного листа"},
                    "product_code": {"type": "STRING", "description": "Код продукции"},
                    "supplier": {"type": "STRING", "description": "Поставщик/Изготовитель"},
                    "unit": {"type": "STRING", "description": "Единица измерения"},
                    "quantity": {"type": "NUMBER", "description": "Количество"},
                    "weight_kg": {"type": "NUMBER", "description": "Масса 1 ед, кг"},
                    "note": {"type": "STRING", "description": "Примечание"}
                },
                "required": ["pos", "name", "quantity", "unit"]
            }
        }
    },
    "required": ["doc_number", "items"]
}

SYSTEM_PROMPT = """
Ты — инженерный ассистент. Твоя задача — извлечь данные спецификации оборудования,
изделий и материалов из текста чертежа ГОСТ и сгруппировать их согласно структуре JSON.

Правила:
- Пропускай заголовки таблиц и служебные надписи (например «Внимание! Перед заказом...»).
- Не придумывай данные, которых нет в тексте.
- Спецификация обычно разбита на разделы («Оборудование», «Кабели и провода»,
  «Изделия и материалы» и т.п.), и нумерация позиций в каждом разделе начинается заново.
  Для КАЖДОЙ позиции указывай её раздел в поле "section". Если позиция явно не под разделом —
  оставь "section" пустым.
- Если внутри одной ячейки текст перенесён на несколько строк, объединяй его через пробел,
  не склеивай слова вместе (например «Пожарной Автоматики», а не «ПожарнойАвтоматики»).
- Поле "weight_kg" (масса) заполняй только если масса реально указана в тексте; если её нет —
  не ставь 0, оставь поле пустым (не указывай).
"""


def get_api_key():
    """Возвращает ключ Gemini из окружения или бросает понятную ошибку."""
    key = os.getenv("GEMINI_API_KEY", "").strip()
    if not key:
        raise RuntimeError(
            "Не найден GEMINI_API_KEY. Создайте .env на основе .env.example "
            "и вставьте ключ (https://aistudio.google.com/apikey)."
        )
    return key


def get_client():
    """Создаёт клиент Gemini."""
    return genai.Client(api_key=get_api_key())


def extract_text_from_pdf(source):
    """Извлекает цифровой текст из векторного PDF.

    source — путь к файлу (str) ИЛИ файловый объект / поток байтов (BytesIO).
    """
    reader = PdfReader(source)
    full_text = []
    for idx, page in enumerate(reader.pages):
        text = page.extract_text()
        if text:
            full_text.append(f"--- СТРАНИЦА {idx + 1} ---\n{text}")
    return "\n\n".join(full_text)


def parse_specification_with_ai(text_content, client=None, max_retries=5):
    """Отправляет текст в Gemini и получает структурированный JSON.

    При временных ошибках сервера (503 — модель перегружена, 429 — лимит)
    повторяет запрос с экспоненциальной задержкой.
    """
    if client is None:
        client = get_client()

    for attempt in range(1, max_retries + 1):
        try:
            response = client.models.generate_content(
                model=GEMINI_MODEL,
                contents=[
                    types.Part.from_text(text=f"Текст спецификации:\n\n{text_content}"),
                ],
                config=types.GenerateContentConfig(
                    system_instruction=SYSTEM_PROMPT,
                    response_mime_type="application/json",
                    response_schema=SPEC_SCHEMA,
                    temperature=0.1
                )
            )
            return json.loads(response.text)
        except Exception as e:
            code = getattr(e, "code", None)
            is_transient = code in (429, 503) or "503" in str(e) or "429" in str(e)
            if not is_transient or attempt == max_retries:
                raise
            time.sleep(2 ** attempt)  # 2, 4, 8, 16 секунд


def items_to_rows(filename, doc_number, system_name, items):
    """Превращает список позиций (items) в список плоских строк для DataFrame."""
    rows = []
    for item in items:
        row = {
            "Имя файла": filename,
            "Шифр документа": doc_number,
            "Система": system_name,
        }
        for header, key in ITEM_COLUMNS:
            value = item.get(key, "")
            if value is None:
                value = ""
            # Масса: не показываем 0 там, где массы нет — оставляем пусто
            if key == "weight_kg" and value in (0, 0.0):
                value = ""
            row[header] = value
        rows.append(row)
    return rows


def safe_sheet_name(filename, used_names):
    """Валидное и уникальное имя листа Excel (макс. 31 символ)."""
    name = os.path.splitext(filename)[0]
    name = re.sub(r'[\[\]:*?/\\]', '_', name)
    name = name[:31]

    original = name
    counter = 1
    while name in used_names:
        suffix = f"_{counter}"
        name = original[:31 - len(suffix)] + suffix
        counter += 1

    used_names.add(name)
    return name


def _write_sheet(worksheet, headers, rows):
    """Пишет заголовки и строки на лист и применяет форматирование (без pandas)."""
    # Шапка
    worksheet.append(headers)

    # Данные — в порядке headers
    for row in rows:
        worksheet.append([row.get(h, "") for h in headers])

    # --- Стилизация шапки ---
    header_font = Font(bold=True, color="FFFFFF")
    header_fill = PatternFill(start_color="4472C4", end_color="4472C4", fill_type="solid")
    header_alignment = Alignment(horizontal="center", vertical="center", wrap_text=True)
    for cell in worksheet[1]:
        cell.font = header_font
        cell.fill = header_fill
        cell.alignment = header_alignment

    # --- Ширина колонок по самому длинному значению ---
    for col_idx, header in enumerate(headers, start=1):
        max_len = len(str(header))
        for row in rows:
            value = row.get(header, "")
            value_len = len(str(value)) if value is not None else 0
            if value_len > max_len:
                max_len = value_len
        width = min(max(max_len + 2, MIN_COLUMN_WIDTH), MAX_COLUMN_WIDTH)
        worksheet.column_dimensions[get_column_letter(col_idx)].width = width

    # --- Перенос текста в данных ---
    for row in worksheet.iter_rows(min_row=2, max_row=worksheet.max_row):
        for cell in row:
            cell.alignment = Alignment(vertical="top", wrap_text=True)

    worksheet.freeze_panes = "A2"
    worksheet.auto_filter.ref = worksheet.dimensions


def build_workbook(file_results, output=None):
    """Собирает Excel-книгу из результатов разбора (на openpyxl, без pandas).

    file_results — список словарей:
        {"filename": str, "doc_number": str, "system_name": str, "items": [ {...}, ... ]}

    output — путь к файлу (str) или None. Если None — вернёт BytesIO с готовой книгой.
    Возвращает путь (str) при записи в файл, иначе BytesIO.
    """
    headers = all_headers()
    all_rows = []
    per_file = []  # (filename, rows)

    for res in file_results:
        rows = items_to_rows(
            res.get("filename", ""),
            res.get("doc_number", ""),
            res.get("system_name", ""),
            res.get("items", []),
        )
        if rows:
            all_rows.extend(rows)
            per_file.append((res.get("filename", "лист"), rows))

    if not all_rows:
        raise ValueError("Нет данных для записи в Excel.")

    wb = Workbook()

    # Сводный лист
    summary_sheet_name = "Сводная спецификация"
    ws_summary = wb.active
    ws_summary.title = summary_sheet_name
    _write_sheet(ws_summary, headers, all_rows)

    # Лист на каждый файл
    used_names = {summary_sheet_name}
    for filename, rows in per_file:
        sheet_name = safe_sheet_name(filename, used_names)
        ws = wb.create_sheet(title=sheet_name)
        _write_sheet(ws, headers, rows)

    target = output if output is not None else io.BytesIO()
    wb.save(target)

    if output is None:
        target.seek(0)
    return target
