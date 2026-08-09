"""
CLI-конвертер: читает PDF из папки input_pdfs/ и собирает сводную Excel-таблицу.
Вся логика вынесена в converter_core.py (её же использует веб-бэкенд).

Запуск:  ./run.sh   или   ./venv/bin/python convert.py
"""

import os

from converter_core import (
    get_client,
    get_api_key,
    extract_text_from_pdf,
    parse_specification_with_ai,
    build_workbook,
)


def main():
    input_dir = "./input_pdfs"
    output_excel = "Сводная_Спецификация_MVP.xlsx"

    # Проверяем ключ сразу, чтобы не падать на середине обработки
    try:
        get_api_key()
    except RuntimeError as e:
        raise SystemExit(f"❌ {e}")

    if not os.path.exists(input_dir):
        os.makedirs(input_dir)
        print(f"📁 Создана папка '{input_dir}'. Положите туда PDF файлы и запустите скрипт снова.")
        return

    pdf_files = [f for f in os.listdir(input_dir) if f.lower().endswith('.pdf')]
    if not pdf_files:
        print(f"⚠️ В папке '{input_dir}' нет PDF файлов.")
        return

    client = get_client()
    file_results = []

    print(f"🚀 Найдено файлов для обработки: {len(pdf_files)}\n")

    for filename in pdf_files:
        pdf_path = os.path.join(input_dir, filename)
        print(f"📄 Обработка файла: {filename}...")

        try:
            pdf_text = extract_text_from_pdf(pdf_path)
            if not pdf_text.strip():
                print(f"  ⚠️ Не удалось извлечь текст из {filename} (возможно, это скан-картинка).")
                continue

            parsed = parse_specification_with_ai(pdf_text, client=client)
            doc_num = parsed.get("doc_number", "Не указан")
            system_name = parsed.get("system_name", "")
            items = parsed.get("items", [])

            print(f"  ✅ Успешно извлечено позиций: {len(items)} (Шифр: {doc_num})")

            if items:
                file_results.append({
                    "filename": filename,
                    "doc_number": doc_num,
                    "system_name": system_name,
                    "items": items,
                })
        except Exception as e:
            print(f"  ❌ Ошибка при обработке {filename}: {e}")

    if file_results:
        build_workbook(file_results, output=output_excel)
        print(f"\n🎉 Все файлы обработаны! Итоговая таблица сохранена в: {output_excel}")
    else:
        print("\n❌ Не удалось извлечь данные ни из одного файла.")


if __name__ == "__main__":
    main()
