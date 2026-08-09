import type { ConvertResponse, FileResult } from './types'

// Загружает PDF-файлы на бэкенд и возвращает разобранные позиции.
export async function convertFiles(files: File[]): Promise<ConvertResponse> {
  const form = new FormData()
  files.forEach((f) => form.append('files', f))

  const res = await fetch('/api/convert', { method: 'POST', body: form })
  if (!res.ok) {
    const detail = await res.json().catch(() => ({}))
    throw new Error(detail.detail || `Ошибка сервера (${res.status})`)
  }
  return res.json()
}

// Отправляет (возможно отредактированные) данные и скачивает готовый Excel.
export async function exportToExcel(results: FileResult[]): Promise<void> {
  const res = await fetch('/api/export', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ results }),
  })
  if (!res.ok) {
    const detail = await res.json().catch(() => ({}))
    throw new Error(detail.detail || `Ошибка экспорта (${res.status})`)
  }

  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'Сводная_Спецификация.xlsx'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

// Проверка доступности бэкенда и настройки ключа.
export async function checkHealth(): Promise<{ key_configured: boolean }> {
  const res = await fetch('/api/health')
  return res.json()
}
