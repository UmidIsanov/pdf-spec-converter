"""
FastAPI-бэкенд конвертера спецификаций.

Эндпоинты:
  GET  /api/health          — проверка, что сервер жив и ключ настроен
  POST /api/convert         — приём PDF (multipart) → JSON с позициями по каждому файлу
  POST /api/export          — приём JSON (возможно отредактированного) → готовый .xlsx

Запуск:  ./backend/run_api.sh   (или uvicorn backend.main:app --reload)
"""

import io
import os
import sys
import uuid
from concurrent.futures import ThreadPoolExecutor
from typing import List, Optional

from fastapi import FastAPI, UploadFile, File
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

# Позволяем импортировать converter_core из корня проекта
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from converter_core import (  # noqa: E402
    get_api_key,
    get_client,
    extract_text_from_pdf,
    parse_specification_with_ai,
    build_workbook,
)

app = FastAPI(title="Конвертер спецификаций", version="1.0.0")

# Разрешаем запросы с фронтенда (Vite dev-сервер и типовые локальные порты)
app.add_middleware(
    CORSMiddleware,
    allow_origins=[
        "http://localhost:5173",
        "http://127.0.0.1:5173",
        "http://localhost:3000",
        "http://127.0.0.1:3000",
    ],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# --- Схемы данных для /api/export ---

class SpecItem(BaseModel):
    section: str = ""
    pos: str = ""
    name: str = ""
    type_code: str = ""
    product_code: str = ""
    supplier: str = ""
    unit: str = ""
    quantity: Optional[float] = None
    weight_kg: Optional[float] = None
    note: str = ""


class FileResult(BaseModel):
    filename: str
    doc_number: str = ""
    object_name: str = ""
    system_name: str = ""
    items: List[SpecItem] = []
    error: Optional[str] = None


class ExportRequest(BaseModel):
    results: List[FileResult]


@app.get("/api/health")
def health():
    """Быстрая проверка: сервер жив, ключ на месте."""
    try:
        get_api_key()
        return {"ok": True, "key_configured": True}
    except RuntimeError as e:
        return {"ok": True, "key_configured": False, "detail": str(e)}


# --- Фоновая обработка задач (обход таймаута Cloudflare ~100 сек) ---
# Один PDF = одна задача. /api/convert мгновенно возвращает job_id и запускает
# разбор в фоновом потоке; фронтенд опрашивает /api/job/{id} каждые 2 сек.
# Так каждый HTTP-запрос короткий, а Gemini работает сколько нужно.
_jobs: dict = {}
_executor = ThreadPoolExecutor(max_workers=4)


def _process_file_job(job_id: str, filename: str, raw: bytes):
    entry = {
        "filename": filename,
        "doc_number": "",
        "object_name": "",
        "system_name": "",
        "items": [],
        "error": None,
    }
    try:
        text = extract_text_from_pdf(io.BytesIO(raw))
        if not text.strip():
            entry["error"] = "Не удалось извлечь текст (возможно, это скан-картинка)."
            _jobs[job_id] = {"status": "done", "result": entry}
            return
        parsed = parse_specification_with_ai(text, client=get_client())
        entry["doc_number"] = parsed.get("doc_number", "")
        entry["object_name"] = parsed.get("object_name", "")
        entry["system_name"] = parsed.get("system_name", "")
        entry["items"] = parsed.get("items", [])
        _jobs[job_id] = {"status": "done", "result": entry}
    except Exception as e:  # noqa: BLE001
        entry["error"] = str(e)
        _jobs[job_id] = {"status": "done", "result": entry}


@app.post("/api/convert")
async def convert(files: List[UploadFile] = File(...)):
    """Запускает фоновый разбор одного PDF, возвращает job_id (мгновенно)."""
    try:
        get_api_key()
    except RuntimeError as e:
        return JSONResponse(status_code=500, content={"detail": str(e)})

    if not files:
        return JSONResponse(status_code=400, content={"detail": "Нет файла."})

    upload = files[0]  # фронтенд шлёт по одному файлу за запрос
    raw = await upload.read()
    job_id = uuid.uuid4().hex
    _jobs[job_id] = {"status": "pending"}
    _executor.submit(_process_file_job, job_id, upload.filename, raw)
    return {"job_id": job_id}


@app.get("/api/job/{job_id}")
def job_status(job_id: str):
    """Статус фоновой задачи: pending | done (+result)."""
    job = _jobs.get(job_id)
    if job is None:
        return JSONResponse(status_code=404, content={"detail": "Задача не найдена."})
    if job.get("status") == "done":
        # отдаём результат и подчищаем, чтобы память не росла
        _jobs.pop(job_id, None)
    return job


@app.post("/api/export")
def export(req: ExportRequest):
    """Собирает Excel из переданных (возможно отредактированных) данных и отдаёт файл."""
    file_results = []
    for r in req.results:
        if r.error or not r.items:
            continue
        file_results.append({
            "filename": r.filename,
            "doc_number": r.doc_number,
            "object_name": r.object_name,
            "system_name": r.system_name,
            "items": [item.model_dump() for item in r.items],
        })

    if not file_results:
        return JSONResponse(status_code=400, content={"detail": "Нет данных для экспорта."})

    try:
        buffer = build_workbook(file_results, output=None)
    except ValueError as e:
        return JSONResponse(status_code=400, content={"detail": str(e)})

    headers = {
        "Content-Disposition": 'attachment; filename="specification.xlsx"'
    }
    return StreamingResponse(
        buffer,
        media_type="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        headers=headers,
    )


# --- Раздача собранного фронтенда (production) ---
# Если рядом есть собранный фронт (frontend/dist), бэкенд отдаёт его сам —
# тогда на сервере достаточно только Python, Node не нужен.
_FRONTEND_DIST = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "frontend", "dist"
)
if os.path.isdir(_FRONTEND_DIST):
    app.mount("/", StaticFiles(directory=_FRONTEND_DIST, html=True), name="frontend")
