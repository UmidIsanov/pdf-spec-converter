import type { FileResult } from './types'

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

// Запускает фоновый разбор одного файла и возвращает job_id.
async function startJob(file: File): Promise<string> {
  const form = new FormData()
  form.append('files', file)
  const res = await fetch('/api/convert', { method: 'POST', body: form })
  if (!res.ok) {
    const detail = await res.json().catch(() => ({}))
    throw new Error(detail.detail || `Ошибка сервера (${res.status})`)
  }
  const data = await res.json()
  return data.job_id
}

// Обрабатывает ОДИН файл: запускает задачу и опрашивает статус каждые 2 сек.
// Каждый запрос короткий — таймаут Cloudflare (~100 сек) не срабатывает,
// а Gemini спокойно работает в фоне сколько нужно.
export async function convertOneFile(file: File): Promise<FileResult> {
  const jobId = await startJob(file)
  // до ~10 минут ожидания на файл (300 опросов × 2 сек)
  for (let i = 0; i < 300; i++) {
    await sleep(2000)
    const res = await fetch(`/api/job/${jobId}`)
    if (!res.ok) {
      if (res.status === 404) throw new Error('Задача потерялась на сервере')
      continue // временная ошибка сети — пробуем ещё
    }
    const job = await res.json()
    if (job.status === 'done') return job.result as FileResult
  }
  throw new Error('Превышено время ожидания обработки файла')
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
