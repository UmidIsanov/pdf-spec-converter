package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// --- Модель Gemini ---
func geminiModel() string {
	if m := os.Getenv("GEMINI_MODEL"); m != "" {
		return m
	}
	return "gemini-3.6-flash"
}

// --- Системный промпт ---
const systemPrompt = `Ты — инженерный ассистент. Твоя задача — извлечь данные спецификации оборудования,
изделий и материалов из чертежа ГОСТ и сгруппировать их согласно структуре JSON.

Правила:
- Пропускай заголовки таблиц и служебные надписи (например «Внимание! Перед заказом...»).
- Не придумывай данные, которых нет в чертеже.
- Спецификация обычно разбита на разделы («Оборудование», «Кабели и провода»,
  «Изделия и материалы» и т.п.), и нумерация позиций в каждом разделе начинается заново.
  Для КАЖДОЙ позиции указывай её раздел в поле "section". Если позиция явно не под разделом —
  оставь "section" пустым.
- Если внутри одной ячейки текст перенесён на несколько строк, объединяй его через пробел,
  не склеивай слова вместе (например «Пожарной Автоматики», а не «ПожарнойАвтоматики»).
- Поле "weight_kg" (масса) заполняй только если масса реально указана; если её нет — не ставь 0.
- В поле "object_name" укажи наименование объекта/сооружения из штампа (то, ГДЕ монтируется
  система), например «Склад крупнодробленой руды» — без названия системы и без стадии.
- ЧИСЛА (количество и масса) переписывай ПРЕДЕЛЬНО ТОЧНО — все цифры до единой, ровно как
  в чертеже. Не пропускай цифры, не округляй, не отбрасывай разряды. Количество бери из
  колонки «Количество», а не из описания (в описании может быть длина одной бухты — это НЕ количество).`

// --- JSON-схема структурированного вывода (для responseSchema) ---
const specSchema = `{
  "type": "OBJECT",
  "properties": {
    "doc_number": {"type": "STRING", "description": "Шифр/номер документа из штампа"},
    "object_name": {"type": "STRING", "description": "Наименование объекта/сооружения из штампа, без названия системы и стадии"},
    "system_name": {"type": "STRING", "description": "Наименование системы/раздела"},
    "items": {
      "type": "ARRAY",
      "items": {
        "type": "OBJECT",
        "properties": {
          "section": {"type": "STRING", "description": "Раздел спецификации (Оборудование / Кабели и провода / Изделия и материалы). Пусто, если раздела нет."},
          "pos": {"type": "STRING", "description": "Позиция/Марка"},
          "name": {"type": "STRING", "description": "Наименование и техническая характеристика"},
          "type_code": {"type": "STRING", "description": "Тип, марка, обозначение"},
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
}`

// --- Структуры данных ---
type SpecItem struct {
	Section     string   `json:"section"`
	Pos         string   `json:"pos"`
	Name        string   `json:"name"`
	TypeCode    string   `json:"type_code"`
	ProductCode string   `json:"product_code"`
	Supplier    string   `json:"supplier"`
	Unit        string   `json:"unit"`
	Quantity    *float64 `json:"quantity"`
	WeightKg    *float64 `json:"weight_kg"`
	Note        string   `json:"note"`
}

type SpecResult struct {
	DocNumber  string     `json:"doc_number"`
	ObjectName string     `json:"object_name"`
	SystemName string     `json:"system_name"`
	Items      []SpecItem `json:"items"`
}

type FileResult struct {
	Filename   string     `json:"filename"`
	DocNumber  string     `json:"doc_number"`
	ObjectName string     `json:"object_name"`
	SystemName string     `json:"system_name"`
	Items      []SpecItem `json:"items"`
	Error      *string    `json:"error"`
}

// --- Колонки ---
var docColumns = []string{"Объект", "Шифр документа", "Система"}

// (заголовок, ключ)
var itemColumns = [][2]string{
	{"Раздел", "section"},
	{"Позиция в спецификации", "pos"},
	{"Наименование и техническая характеристика", "name"},
	{"Тип, марка, обозначение", "type_code"},
	{"Код продукции", "product_code"},
	{"Поставщик", "supplier"},
	{"Ед. изм.", "unit"},
	{"Кол-во", "quantity"},
	{"Примечание", "note"},
}

func allHeaders() []string {
	h := append([]string{}, docColumns...)
	for _, c := range itemColumns {
		h = append(h, c[0])
	}
	return h
}

// summaryColumns — лист «Итого по наименованиям»
var summaryColumns = []string{
	"Наименование и техническая характеристика",
	"Тип, марка, обозначение",
	"Код продукции",
	"Ед. изм.",
	"Итого количество",
	"Кол-во объектов",
}

// --- Хелперы ---

func fullShifr(filename string) string {
	name := filepath.Base(filename)
	ext := filepath.Ext(name)
	return strings.TrimSpace(strings.TrimSuffix(name, ext))
}

var reSysCode = regexp.MustCompile(`([A-Z]{2,5})-\d`)

func systemCode(shifr string) string {
	m := reSysCode.FindAllStringSubmatch(shifr, -1)
	if len(m) == 0 {
		return ""
	}
	return m[len(m)-1][1]
}

func norm(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func numToStr(q *float64) string {
	if q == nil {
		return ""
	}
	if *q == float64(int64(*q)) {
		return strconv.FormatInt(int64(*q), 10)
	}
	return strconv.FormatFloat(*q, 'g', -1, 64)
}

// valueForKey — значение позиции по ключу колонки (строкой)
func (it SpecItem) valueForKey(key string) string {
	switch key {
	case "section":
		return it.Section
	case "pos":
		return it.Pos
	case "name":
		return it.Name
	case "type_code":
		return it.TypeCode
	case "product_code":
		return it.ProductCode
	case "supplier":
		return it.Supplier
	case "unit":
		return it.Unit
	case "quantity":
		return numToStr(it.Quantity)
	case "note":
		return it.Note
	}
	return ""
}

// rowsForFile — строки одного файла в порядке allHeaders()
func rowsForFile(fr FileResult) [][]string {
	shifr := fullShifr(fr.Filename)
	obj := strings.TrimSpace(firstNonEmpty(fr.ObjectName, fr.SystemName))
	sys := systemCode(shifr)
	if sys == "" {
		sys = fr.SystemName
	}
	var rows [][]string
	for _, it := range fr.Items {
		row := []string{obj, shifr, sys}
		for _, c := range itemColumns {
			row = append(row, it.valueForKey(c[1]))
		}
		rows = append(rows, row)
	}
	return rows
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// summarizeByName — сводка: сумма количества по коду товара (иначе по имени)
func summarizeByName(files []FileResult) [][]string {
	type grp struct {
		name, typeCode, prodCode, unit string
		qty                            float64
		objects                        map[string]bool
	}
	groups := map[string]*grp{}
	for _, fr := range files {
		if fr.Error != nil {
			continue
		}
		shifr := fullShifr(fr.Filename)
		for _, it := range fr.Items {
			name := strings.TrimSpace(it.Name)
			if name == "" {
				continue
			}
			code := strings.TrimSpace(firstNonEmpty(it.ProductCode, it.TypeCode))
			var key string
			if code != "" {
				key = "c:" + norm(code)
			} else {
				key = "n:" + norm(name)
			}
			key += "|" + norm(it.Unit)
			g := groups[key]
			if g == nil {
				g = &grp{name: name, typeCode: it.TypeCode, prodCode: it.ProductCode, unit: it.Unit, objects: map[string]bool{}}
				groups[key] = g
			}
			if it.Quantity != nil {
				g.qty += *it.Quantity
			}
			if len(name) > len(g.name) {
				g.name = name
			}
			if g.typeCode == "" {
				g.typeCode = it.TypeCode
			}
			if g.prodCode == "" {
				g.prodCode = it.ProductCode
			}
			g.objects[shifr] = true
		}
	}
	var rows [][]string
	for _, g := range groups {
		qtyStr := strconv.FormatFloat(g.qty, 'g', -1, 64)
		if g.qty == float64(int64(g.qty)) {
			qtyStr = strconv.FormatInt(int64(g.qty), 10)
		}
		rows = append(rows, []string{g.name, g.typeCode, g.prodCode, g.unit, qtyStr, strconv.Itoa(len(g.objects))})
	}
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i][0]) < strings.ToLower(rows[j][0])
	})
	return rows
}
