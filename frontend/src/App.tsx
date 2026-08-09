import { useCallback, useEffect, useMemo, useState } from 'react'
import { useDropzone } from 'react-dropzone'
import {
  FileText,
  UploadCloud,
  X,
  Loader2,
  Download,
  Sparkles,
  AlertTriangle,
  CheckCircle2,
  FileSpreadsheet,
  ChevronDown,
  Search,
  Calculator,
} from 'lucide-react'
import { convertFiles, exportToExcel, checkHealth } from './api'
import { ITEM_COLUMNS, type FileResult } from './types'

type Status = 'idle' | 'loading' | 'done'

export default function App() {
  const [files, setFiles] = useState<File[]>([])
  const [status, setStatus] = useState<Status>('idle')
  const [results, setResults] = useState<FileResult[]>([])
  const [error, setError] = useState<string | null>(null)
  const [exporting, setExporting] = useState(false)
  const [keyMissing, setKeyMissing] = useState(false)
  const [progress, setProgress] = useState({ done: 0, total: 0 })

  useEffect(() => {
    checkHealth()
      .then((h) => setKeyMissing(!h.key_configured))
      .catch(() => setKeyMissing(false))
  }, [])

  const onDrop = useCallback((accepted: File[]) => {
    setError(null)
    setFiles((prev) => {
      const existing = new Set(prev.map((f) => f.name + f.size))
      const fresh = accepted.filter((f) => !existing.has(f.name + f.size))
      return [...prev, ...fresh]
    })
  }, [])

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: { 'application/pdf': ['.pdf'] },
    multiple: true,
  })

  const removeFile = (name: string, size: number) =>
    setFiles((prev) => prev.filter((f) => !(f.name === name && f.size === size)))

  const handleConvert = async () => {
    if (!files.length) return
    setStatus('loading')
    setError(null)
    setProgress({ done: 0, total: files.length })

    // Обрабатываем файлы ПО ОДНОМУ отдельными запросами:
    // так каждый запрос короткий и не упирается в таймаут Cloudflare (~100 сек).
    const acc: FileResult[] = []
    try {
      for (const f of files) {
        const res = await convertFiles([f])
        acc.push(...res.results)
        setProgress({ done: acc.length, total: files.length })
      }
      setResults(acc)
      setStatus('done')
    } catch (e) {
      // Если что-то упало посреди пачки — показываем уже обработанное + ошибку
      if (acc.length > 0) {
        setResults(acc)
        setStatus('done')
        setError(
          `Обработано ${acc.length} из ${files.length}. На остальных ошибка: ${
            e instanceof Error ? e.message : 'неизвестная'
          }`,
        )
      } else {
        setError(e instanceof Error ? e.message : 'Неизвестная ошибка')
        setStatus('idle')
      }
    }
  }

  const handleExport = async () => {
    setExporting(true)
    setError(null)
    try {
      await exportToExcel(results)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ошибка экспорта')
    } finally {
      setExporting(false)
    }
  }

  const reset = () => {
    setFiles([])
    setResults([])
    setStatus('idle')
    setError(null)
  }

  const totalItems = useMemo(
    () => results.reduce((sum, r) => sum + r.items.length, 0),
    [results],
  )
  const okFiles = results.filter((r) => !r.error && r.items.length > 0).length

  return (
    <div className="min-h-full bg-slate-50 text-slate-900">
      {/* Шапка */}
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto max-w-5xl px-6 py-5 flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-blue-600 text-white flex items-center justify-center shadow-lg shadow-blue-100">
            <FileSpreadsheet size={22} />
          </div>
          <div>
            <h1 className="font-black text-lg leading-none text-slate-800">
              Конвертер спецификаций
            </h1>
            <p className="text-[11px] text-slate-400 mt-1 uppercase tracking-widest font-bold">
              PDF (ГОСТ) → Excel · на базе Gemini
            </p>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-6 py-8 space-y-6">
        {keyMissing && (
          <Banner
            icon={<AlertTriangle size={18} />}
            tone="amber"
            text="API-ключ Gemini не настроен на сервере. Добавьте GEMINI_API_KEY в файл .env и перезапустите бэкенд."
          />
        )}
        {error && (
          <Banner icon={<AlertTriangle size={18} />} tone="red" text={error} />
        )}

        {/* Зона загрузки */}
        {status !== 'done' && (
          <div
            {...getRootProps()}
            className={`rounded-3xl border-2 border-dashed p-12 text-center cursor-pointer transition-all ${
              isDragActive
                ? 'border-blue-500 bg-blue-50'
                : 'border-slate-300 bg-white hover:border-blue-400 hover:bg-slate-50'
            }`}
          >
            <input {...getInputProps()} />
            <div className="w-16 h-16 rounded-2xl bg-blue-100 text-blue-600 flex items-center justify-center mx-auto mb-5">
              <UploadCloud size={32} />
            </div>
            <p className="font-black text-slate-800 text-lg">
              {isDragActive
                ? 'Отпустите файлы здесь'
                : 'Перетащите PDF-чертежи сюда'}
            </p>
            <p className="text-sm text-slate-500 font-medium mt-2">
              или нажмите, чтобы выбрать файлы · можно несколько сразу
            </p>
          </div>
        )}

        {/* Список выбранных файлов */}
        {status !== 'done' && files.length > 0 && (
          <div className="bg-white rounded-3xl border border-slate-200 p-5 space-y-3">
            <p className="text-[11px] font-black text-slate-400 uppercase tracking-widest">
              Выбрано файлов: {files.length}
            </p>
            <div className="space-y-2">
              {files.map((f) => (
                <div
                  key={f.name + f.size}
                  className="flex items-center gap-3 p-3 rounded-2xl bg-slate-50 border border-slate-100"
                >
                  <div className="w-9 h-9 rounded-xl bg-red-50 text-red-500 flex items-center justify-center shrink-0">
                    <FileText size={18} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-bold text-slate-700 truncate">
                      {f.name}
                    </p>
                    <p className="text-[11px] text-slate-400 font-medium">
                      {(f.size / 1024 / 1024).toFixed(2)} МБ
                    </p>
                  </div>
                  <button
                    onClick={() => removeFile(f.name, f.size)}
                    className="p-2 text-slate-400 hover:text-red-500 transition-colors"
                    disabled={status === 'loading'}
                  >
                    <X size={18} />
                  </button>
                </div>
              ))}
            </div>

            <button
              onClick={handleConvert}
              disabled={status === 'loading'}
              className="w-full mt-2 bg-blue-600 text-white py-4 rounded-2xl font-black uppercase tracking-widest shadow-lg shadow-blue-500/20 active:scale-[0.99] transition-all flex items-center justify-center gap-2 disabled:bg-slate-300 disabled:shadow-none"
            >
              {status === 'loading' ? (
                <>
                  <Loader2 size={20} className="animate-spin" />
                  {progress.total > 1
                    ? `Обработка ${progress.done}/${progress.total}…`
                    : 'Обработка через Gemini…'}
                </>
              ) : (
                <>
                  <Sparkles size={20} /> Конвертировать
                </>
              )}
            </button>
          </div>
        )}

        {/* Результаты */}
        {status === 'done' && (
          <div className="space-y-6">
            <div className="bg-white rounded-3xl border border-slate-200 p-5 flex flex-wrap items-center gap-4 justify-between">
              <div className="flex items-center gap-4">
                <div className="w-12 h-12 rounded-2xl bg-green-100 text-green-600 flex items-center justify-center">
                  <CheckCircle2 size={26} />
                </div>
                <div>
                  <p className="font-black text-slate-800 text-lg leading-none">
                    Готово · {totalItems}{' '}
                    {totalItems === 1 ? 'позиция' : 'позиций'}
                  </p>
                  <p className="text-sm text-slate-500 font-medium mt-1">
                    Успешно обработано файлов: {okFiles} из {results.length}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-3">
                <button
                  onClick={reset}
                  className="px-5 py-3 rounded-2xl font-bold text-slate-600 bg-slate-100 hover:bg-slate-200 transition-colors"
                >
                  Новый разбор
                </button>
                <button
                  onClick={handleExport}
                  disabled={exporting || totalItems === 0}
                  className="px-6 py-3 rounded-2xl font-black uppercase tracking-widest text-white bg-green-600 shadow-lg shadow-green-500/20 active:scale-[0.99] transition-all flex items-center gap-2 disabled:bg-slate-300 disabled:shadow-none"
                >
                  {exporting ? (
                    <Loader2 size={18} className="animate-spin" />
                  ) : (
                    <Download size={18} />
                  )}
                  Скачать Excel
                </button>
              </div>
            </div>

            <SearchSum results={results} />

            {results.map((r) => (
              <FileResultCard key={r.filename} result={r} />
            ))}
          </div>
        )}
      </main>
    </div>
  )
}

function FileResultCard({ result }: { result: FileResult }) {
  const [open, setOpen] = useState(true)
  const hasError = !!result.error
  const empty = !hasError && result.items.length === 0

  return (
    <div className="bg-white rounded-3xl border border-slate-200 overflow-hidden">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full p-5 flex items-center gap-4 text-left"
      >
        <div
          className={`w-11 h-11 rounded-2xl flex items-center justify-center shrink-0 ${
            hasError
              ? 'bg-red-50 text-red-500'
              : 'bg-blue-50 text-blue-600'
          }`}
        >
          <FileText size={22} />
        </div>
        <div className="flex-1 min-w-0">
          <p className="font-bold text-slate-800 truncate">{result.filename}</p>
          <p className="text-[11px] text-slate-400 font-bold uppercase tracking-wider mt-0.5">
            {hasError
              ? 'Ошибка'
              : `${result.doc_number || 'Шифр не найден'} · ${result.items.length} поз.`}
          </p>
        </div>
        {!hasError && !empty && (
          <ChevronDown
            size={20}
            className={`text-slate-300 transition-transform ${open ? 'rotate-180' : ''}`}
          />
        )}
      </button>

      {hasError && (
        <div className="px-5 pb-5">
          <div className="p-4 rounded-2xl bg-red-50 border border-red-100 text-sm font-medium text-red-700">
            {result.error}
          </div>
        </div>
      )}

      {empty && (
        <div className="px-5 pb-5 text-sm text-slate-500 font-medium">
          Позиции не найдены.
        </div>
      )}

      {!hasError && !empty && open && (
        <div className="overflow-x-auto border-t border-slate-100">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-slate-50 text-slate-500">
                {ITEM_COLUMNS.map((c) => (
                  <th
                    key={c.key}
                    className="text-left font-black uppercase tracking-wider text-[10px] px-4 py-3 whitespace-nowrap"
                  >
                    {c.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.items.map((item, idx) => (
                <tr
                  key={idx}
                  className="border-t border-slate-50 hover:bg-slate-50/60 align-top"
                >
                  {ITEM_COLUMNS.map((c) => (
                    <td
                      key={c.key}
                      className={`px-4 py-3 font-medium text-slate-700 ${
                        c.key === 'name' ? 'min-w-[260px]' : 'whitespace-nowrap'
                      }`}
                    >
                      {item[c.key] === null || item[c.key] === ''
                        ? '—'
                        : String(item[c.key])}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function SearchSum({ results }: { results: FileResult[] }) {
  const [query, setQuery] = useState('')

  // Совпадения по всем файлам: позиция подходит, если ВСЕ слова запроса
  // встречаются в её названии/марке/коде (пословный поиск — прощает порядок и вставки)
  const matches = useMemo(() => {
    const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
    if (terms.length === 0) return []
    const out: {
      filename: string
      name: string
      type_code: string
      product_code: string
      quantity: number | null
      unit: string
    }[] = []
    for (const r of results) {
      if (r.error) continue
      for (const it of r.items) {
        const hay = `${it.name} ${it.type_code} ${it.product_code}`.toLowerCase()
        if (terms.every((t) => hay.includes(t))) {
          out.push({
            filename: r.filename,
            name: it.name,
            type_code: it.type_code,
            product_code: it.product_code,
            quantity: it.quantity,
            unit: it.unit,
          })
        }
      }
    }
    return out
  }, [query, results])

  // Итоговое количество, сгруппированное по единице измерения
  const totals = useMemo(() => {
    const m = new Map<string, number>()
    for (const it of matches) {
      const unit = it.unit || 'шт.'
      const qty = typeof it.quantity === 'number' ? it.quantity : 0
      m.set(unit, (m.get(unit) || 0) + qty)
    }
    return Array.from(m.entries())
  }, [matches])

  return (
    <div className="bg-white rounded-3xl border border-slate-200 p-5">
      <div className="flex items-center gap-3 mb-4">
        <div className="w-10 h-10 rounded-2xl bg-indigo-100 text-indigo-600 flex items-center justify-center shrink-0">
          <Calculator size={20} />
        </div>
        <div>
          <p className="font-black text-slate-800 leading-none">
            Поиск и подсчёт по всем файлам
          </p>
          <p className="text-xs text-slate-500 font-medium mt-1">
            Введите код или название — посчитаю суммарное количество
          </p>
        </div>
      </div>

      <div className="relative">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Например: RBZ-337936  или  Извещатель дымовой ИП 212-64"
          className="w-full bg-slate-50 border border-slate-200 rounded-2xl py-3 pl-11 pr-4 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all"
        />
        <Search
          size={18}
          className="absolute left-4 top-1/2 -translate-y-1/2 text-slate-400"
        />
      </div>

      {query.trim() && (
        <div className="mt-4">
          {matches.length === 0 ? (
            <p className="text-sm text-slate-500 font-medium">Ничего не найдено.</p>
          ) : (
            <>
              <div className="flex flex-wrap gap-2 mb-3">
                {totals.map(([unit, sum]) => (
                  <div
                    key={unit}
                    className="bg-green-50 text-green-700 px-4 py-2 rounded-2xl font-black text-sm border border-green-100"
                  >
                    Итого: {sum} {unit}
                  </div>
                ))}
                <div className="bg-slate-100 text-slate-600 px-4 py-2 rounded-2xl font-bold text-sm">
                  Совпадений: {matches.length}
                </div>
              </div>

              <div className="overflow-x-auto border-t border-slate-100">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-slate-500">
                      <th className="text-left font-black uppercase tracking-wider text-[10px] px-3 py-2 whitespace-nowrap">
                        Файл
                      </th>
                      <th className="text-left font-black uppercase tracking-wider text-[10px] px-3 py-2">
                        Наименование
                      </th>
                      <th className="text-left font-black uppercase tracking-wider text-[10px] px-3 py-2 whitespace-nowrap">
                        Марка / код
                      </th>
                      <th className="text-right font-black uppercase tracking-wider text-[10px] px-3 py-2 whitespace-nowrap">
                        Кол-во
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {matches.map((it, i) => (
                      <tr key={i} className="border-t border-slate-50 align-top">
                        <td className="px-3 py-2 text-slate-500 whitespace-nowrap">
                          {it.filename.replace(/\.pdf$/i, '')}
                        </td>
                        <td className="px-3 py-2 font-medium text-slate-700 min-w-[240px]">
                          {it.name}
                        </td>
                        <td className="px-3 py-2 text-slate-600 whitespace-nowrap">
                          {it.type_code || it.product_code || '—'}
                        </td>
                        <td className="px-3 py-2 font-bold text-slate-800 text-right whitespace-nowrap">
                          {it.quantity ?? '—'} {it.unit}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}

function Banner({
  icon,
  text,
  tone,
}: {
  icon: React.ReactNode
  text: string
  tone: 'amber' | 'red'
}) {
  const tones = {
    amber: 'bg-amber-50 border-amber-200 text-amber-800',
    red: 'bg-red-50 border-red-200 text-red-700',
  }
  return (
    <div
      className={`rounded-2xl border p-4 flex items-start gap-3 text-sm font-medium ${tones[tone]}`}
    >
      <span className="shrink-0 mt-0.5">{icon}</span>
      <span>{text}</span>
    </div>
  )
}
