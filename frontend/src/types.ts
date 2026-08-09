// Одна позиция спецификации (соответствует SpecItem на бэкенде)
export interface SpecItem {
  section: string
  pos: string
  name: string
  type_code: string
  product_code: string
  supplier: string
  unit: string
  quantity: number | null
  weight_kg: number | null
  note: string
}

// Результат разбора одного PDF-файла
export interface FileResult {
  filename: string
  doc_number: string
  system_name: string
  items: SpecItem[]
  error: string | null
}

export interface ConvertResponse {
  results: FileResult[]
}

// Колонки таблицы: ключ в SpecItem + подпись
export const ITEM_COLUMNS: { key: keyof SpecItem; label: string }[] = [
  { key: 'section', label: 'Раздел' },
  { key: 'pos', label: 'Поз.' },
  { key: 'name', label: 'Наименование и тех. характеристика' },
  { key: 'type_code', label: 'Тип, марка' },
  { key: 'product_code', label: 'Код продукции' },
  { key: 'supplier', label: 'Поставщик' },
  { key: 'unit', label: 'Ед. изм.' },
  { key: 'quantity', label: 'Кол-во' },
  { key: 'weight_kg', label: 'Масса, кг' },
  { key: 'note', label: 'Примечание' },
]
