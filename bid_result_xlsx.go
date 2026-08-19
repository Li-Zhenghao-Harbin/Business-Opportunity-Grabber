package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type bidPackageRow struct {
	SectionName string
	SectionNo   string
	PackageNo   string
	Amount      float64
	Quantity    float64
}

var xmlInvalidRE = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F]`)

func writeBidResultWorkbook(archivePath string, item Opportunity, body string, attachments []Attachment) error {
	rows, notes := collectBidPackageRows(archivePath, attachments)
	if strings.Contains(item.Title, "变更公告") {
		notes = append(notes, "变更公告：请对照原表复核取消招标、金额或标包变化；变更记录以红色标记。")
	}
	if len(rows) == 0 {
		notes = append(notes, "未从公告附件识别到需求一览表或货物清单，请人工复核已下载文件。")
	}
	return writeSimpleXLSX(filepath.Join(archivePath, "招标及结果.xlsx"), item, body, rows, notes)
}

func bidResultWorkbookNeedsRefresh(path string, attachments []Attachment) bool {
	hasZIP := false
	for _, attachment := range attachments {
		if attachment.Status == "已下载" && strings.EqualFold(filepath.Ext(attachment.LocalPath), ".zip") {
			hasZIP = true
			break
		}
	}
	if !hasZIP {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return true
	}
	for _, file := range reader.File {
		if file.Name != "xl/worksheets/sheet2.xml" {
			continue
		}
		content, err := readZipFile(file)
		if err != nil {
			return true
		}
		return bytes.Count(content, []byte("<row ")) <= 1
	}
	return true
}

func collectBidPackageRows(archivePath string, attachments []Attachment) ([]bidPackageRow, []string) {
	var rows []bidPackageRow
	var notes []string
	for _, attachment := range attachments {
		if attachment.Status != "已下载" || attachment.LocalPath == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(attachment.LocalPath))
		switch ext {
		case ".zip":
			extracted, err := unzipBidAttachment(attachment.LocalPath, archivePath)
			if err != nil {
				notes = append(notes, attachment.Name+" 解压失败："+err.Error())
				continue
			}
			for _, path := range extracted {
				if strings.EqualFold(filepath.Ext(path), ".xlsx") {
					parsed, note := readBidRowsFromXLSX(path)
					rows = append(rows, parsed...)
					if note != "" {
						notes = append(notes, filepath.Base(path)+"："+note)
					}
				}
			}
		case ".xlsx":
			parsed, note := readBidRowsFromXLSX(attachment.LocalPath)
			rows = append(rows, parsed...)
			if note != "" {
				notes = append(notes, attachment.Name+"："+note)
			}
		case ".xls":
			notes = append(notes, attachment.Name+" 为旧版 XLS，需人工复核。")
		}
	}
	return mergeBidPackageRows(rows), notes
}

func unzipBidAttachment(zipPath, archivePath string) ([]string, error) {
	return unzipBidAttachmentAtDepth(zipPath, archivePath, 0)
}

func unzipBidAttachmentAtDepth(zipPath, archivePath string, depth int) ([]string, error) {
	if depth >= 4 {
		return nil, fmt.Errorf("压缩包嵌套层级超过 4 层")
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	folderName := strings.TrimSuffix(filepath.Base(zipPath), filepath.Ext(zipPath))
	folderName = trimFileName(sanitizeFileName(folderName), 120)
	if folderName == "" {
		folderName = "公告文件"
	}
	base := filepath.Join(archivePath, folderName)
	if err := os.MkdirAll(base, 0755); err != nil {
		return nil, err
	}
	var paths []string
	var nestedZIPs []string
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := strings.ReplaceAll(decodeZIPEntryName(file.Name, file.NonUTF8), "\\", "/")
		name = path.Clean(name)
		if name == "." || name == "" || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
			continue
		}
		extractedPath := filepath.Join(base, filepath.FromSlash(name))
		if !isWithinDirectory(extractedPath, base) {
			return nil, fmt.Errorf("压缩包包含不安全的文件路径：%s", file.Name)
		}
		if err := os.MkdirAll(filepath.Dir(extractedPath), 0755); err != nil {
			return nil, err
		}
		in, err := file.Open()
		if err != nil {
			return nil, err
		}
		out, err := os.Create(extractedPath)
		if err == nil {
			_, err = io.Copy(out, io.LimitReader(in, 128*1024*1024))
			closeErr := out.Close()
			if err == nil {
				err = closeErr
			}
		}
		in.Close()
		if err != nil {
			return nil, err
		}
		paths = append(paths, extractedPath)
		if strings.EqualFold(filepath.Ext(extractedPath), ".zip") {
			nestedZIPs = append(nestedZIPs, extractedPath)
		}
	}
	for _, nestedZIP := range nestedZIPs {
		nestedPaths, err := unzipBidAttachmentAtDepth(nestedZIP, filepath.Dir(nestedZIP), depth+1)
		if err != nil {
			return nil, err
		}
		paths = append(paths, nestedPaths...)
	}
	return paths, nil
}

func decodeZIPEntryName(name string, nonUTF8 bool) string {
	if !nonUTF8 {
		return name
	}
	decoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), []byte(name))
	if err != nil || strings.TrimSpace(string(decoded)) == "" {
		return name
	}
	return string(decoded)
}

func readBidRowsFromXLSX(path string) ([]bidPackageRow, string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err.Error()
	}
	return readBidRowsFromXLSXBytes(raw)
}

func readBidRowsFromXLSXBytes(raw []byte) ([]bidPackageRow, string) {
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, "不是可识别的 XLSX 文件"
	}
	shared := []string{}
	sheets := map[string][]string{}
	for _, file := range reader.File {
		content, readErr := readZipFile(file)
		if readErr != nil {
			continue
		}
		switch file.Name {
		case "xl/sharedStrings.xml":
			shared = parseSharedStrings(content)
		default:
			if strings.HasPrefix(file.Name, "xl/worksheets/") && strings.HasSuffix(file.Name, ".xml") {
				sheets[file.Name] = parseSheetRows(content, shared)
			}
		}
	}
	var result []bidPackageRow
	for _, sheet := range sheets {
		result = append(result, extractBidRows(sheet)...)
	}
	if len(result) == 0 {
		return nil, "未识别到包含分标、包号、金额或数量列的需求清单"
	}
	return result, ""
}

func readZipFile(file *zip.File) ([]byte, error) {
	in, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return io.ReadAll(io.LimitReader(in, 64*1024*1024))
}

func parseSharedStrings(raw []byte) []string {
	var document struct {
		Items []struct {
			Text []string `xml:"t"`
		} `xml:"si"`
	}
	if xml.Unmarshal(raw, &document) != nil {
		return nil
	}
	values := make([]string, len(document.Items))
	for i, item := range document.Items {
		values[i] = strings.Join(item.Text, "")
	}
	return values
}

func parseSheetRows(raw []byte, shared []string) []string {
	var document struct {
		Rows []struct {
			Cells []struct {
				Ref  string `xml:"r,attr"`
				Type string `xml:"t,attr"`
				Val  string `xml:"v"`
				Text string `xml:"is>t"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if xml.Unmarshal(raw, &document) != nil {
		return nil
	}
	rows := make([]string, 0, len(document.Rows))
	for _, row := range document.Rows {
		values := make([]string, 0, len(row.Cells))
		for _, cell := range row.Cells {
			value := cell.Val
			if cell.Type == "s" {
				index, _ := strconv.Atoi(value)
				if index >= 0 && index < len(shared) {
					value = shared[index]
				}
			}
			if cell.Type == "inlineStr" {
				value = cell.Text
			}
			values = append(values, normalizeText(value))
		}
		rows = append(rows, strings.Join(values, "\t"))
	}
	return rows
}

func extractBidRows(rows []string) []bidPackageRow {
	sectionName := sectionNameFromRows(rows)
	for index, row := range rows {
		headings := strings.Split(row, "\t")
		columns := bidColumns(headings)
		if columns.packageNo < 0 || (columns.amount < 0 && columns.quantity < 0) {
			continue
		}
		var result []bidPackageRow
		for _, dataRow := range rows[index+1:] {
			values := strings.Split(dataRow, "\t")
			packageCode := cellAt(values, columns.packageNo)
			if packageCode == "" {
				continue
			}
			rowSectionName := cellAt(values, columns.sectionName)
			rowSectionNo := cellAt(values, columns.sectionNo)
			packageNo := packageCode
			parsedSectionNo, parsedPackageNo := parseBidPackageCode(packageCode)
			if rowSectionName == "" {
				rowSectionName = sectionName
			}
			if rowSectionNo == "" {
				rowSectionNo = parsedSectionNo
			}
			if parsedPackageNo != "" {
				packageNo = parsedPackageNo
			}
			result = append(result, bidPackageRow{
				SectionName: rowSectionName,
				SectionNo:   rowSectionNo,
				PackageNo:   packageNo,
				Amount:      numberAt(values, columns.amount),
				Quantity:    numberAt(values, columns.quantity),
			})
		}
		return result
	}
	return nil
}

func sectionNameFromRows(rows []string) string {
	for _, row := range rows {
		for _, value := range strings.Split(row, "\t") {
			match := regexp.MustCompile(`[（(]([^（）()]+标段)[）)]`).FindStringSubmatch(value)
			if len(match) == 2 {
				return normalizeText(match[1])
			}
		}
	}
	return ""
}

func parseBidPackageCode(value string) (string, string) {
	value = normalizeText(value)
	match := regexp.MustCompile(`^.+?-([^-]+)-[^-]+-(包[^\s]+)$`).FindStringSubmatch(value)
	if len(match) == 3 {
		return match[1], match[2]
	}
	return "", ""
}

type bidColumnIndexes struct{ sectionName, sectionNo, packageNo, amount, quantity int }

func bidColumns(headings []string) bidColumnIndexes {
	result := bidColumnIndexes{-1, -1, -1, -1, -1}
	for index, heading := range headings {
		value := strings.ReplaceAll(strings.ToLower(heading), " ", "")
		switch {
		case strings.Contains(value, "分标名称"):
			result.sectionName = index
		case strings.Contains(value, "分标编号"):
			result.sectionNo = index
		case strings.Contains(value, "包号") || strings.Contains(value, "包编号") || strings.Contains(value, "分包编号"):
			result.packageNo = index
		case strings.Contains(value, "估算金额") || strings.Contains(value, "预估金额") || strings.Contains(value, "最高限价") || strings.Contains(value, "金额"):
			result.amount = index
		case strings.Contains(value, "数量"):
			result.quantity = index
		}
	}
	return result
}

func cellAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func numberAt(values []string, index int) float64 {
	value := strings.ReplaceAll(cellAt(values, index), ",", "")
	value = regexp.MustCompile(`[^0-9.\-]`).ReplaceAllString(value, "")
	result, _ := strconv.ParseFloat(value, 64)
	return result
}

func mergeBidPackageRows(rows []bidPackageRow) []bidPackageRow {
	merged := map[string]bidPackageRow{}
	for _, row := range rows {
		key := strings.Join([]string{row.SectionName, row.SectionNo, row.PackageNo}, "\x00")
		current := merged[key]
		current.SectionName, current.SectionNo, current.PackageNo = row.SectionName, row.SectionNo, row.PackageNo
		current.Amount += row.Amount
		current.Quantity += row.Quantity
		merged[key] = current
	}
	result := make([]bidPackageRow, 0, len(merged))
	for _, row := range merged {
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SectionName+result[i].SectionNo+result[i].PackageNo < result[j].SectionName+result[j].SectionNo+result[j].PackageNo
	})
	return result
}

func writeSimpleXLSX(path string, item Opportunity, body string, rows []bidPackageRow, notes []string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	defer writer.Close()
	projectRows := [][]string{
		{"采购项目编号", "采购项目名称", "采购类型", "招标文件获取截止时间", "开标（截标）时间", "招标人"},
		{item.TenderNo, item.Title, item.NoticeType, item.Deadline, extractDeadline(body), item.Buyer},
	}
	packageRows := [][]string{{"分标名称", "分标编号", "包号", "估算金额", "数量", "中标候选人"}}
	for _, row := range rows {
		packageRows = append(packageRows, []string{row.SectionName, row.SectionNo, row.PackageNo, strconv.FormatFloat(row.Amount, 'f', -1, 64), strconv.FormatFloat(row.Quantity, 'f', -1, 64), ""})
	}
	noteRows := [][]string{{"处理说明"}}
	for _, note := range notes {
		noteRows = append(noteRows, []string{note})
	}
	parts := map[string]string{
		"[Content_Types].xml":        xlsxContentTypes,
		"_rels/.rels":                xlsxRootRels,
		"xl/workbook.xml":            xlsxWorkbook,
		"xl/_rels/workbook.xml.rels": xlsxWorkbookRels,
		"xl/styles.xml":              xlsxStyles,
		"xl/worksheets/sheet1.xml":   xlsxSheetXML(projectRows, false),
		"xl/worksheets/sheet2.xml":   xlsxSheetXML(packageRows, false),
		"xl/worksheets/sheet3.xml":   xlsxSheetXML(noteRows, strings.Contains(item.Title, "变更公告")),
	}
	for name, content := range parts {
		entry, createErr := writer.Create(name)
		if createErr != nil {
			return createErr
		}
		if _, writeErr := entry.Write([]byte(content)); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func extractDeadline(body string) string {
	match := regexp.MustCompile(`(?:开标|截标|应答文件).*?(\d{4}[-年/]\d{1,2}[-月/]\d{1,2}[^\n]{0,24})`).FindStringSubmatch(body)
	if len(match) > 1 {
		return normalizeText(match[1])
	}
	return ""
}

func xlsxSheetXML(rows [][]string, red bool) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIndex, row := range rows {
		builder.WriteString(fmt.Sprintf(`<row r="%d">`, rowIndex+1))
		for columnIndex, value := range row {
			style := ""
			if rowIndex == 0 || red {
				style = ` s="1"`
			}
			builder.WriteString(fmt.Sprintf(`<c r="%s%d" t="inlineStr"%s><is><t>%s</t></is></c>`, xlsxColumnName(columnIndex+1), rowIndex+1, style, xmlText(value)))
		}
		builder.WriteString(`</row>`)
	}
	builder.WriteString(`</sheetData></worksheet>`)
	return builder.String()
}

func xlsxColumnName(index int) string {
	name := ""
	for index > 0 {
		index--
		name = string(rune('A'+index%26)) + name
		index /= 26
	}
	return name
}

func xmlText(value string) string {
	value = xmlInvalidRE.ReplaceAllString(value, "")
	return html.EscapeString(value)
}

const xlsxContentTypes = `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet3.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`
const xlsxRootRels = `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`
const xlsxWorkbook = `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="招标及结果" sheetId="1" r:id="rId1"/><sheet name="标包需求" sheetId="2" r:id="rId2"/><sheet name="处理说明" sheetId="3" r:id="rId3"/></sheets></workbook>`
const xlsxWorkbookRels = `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet3.xml"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`
const xlsxStyles = `<?xml version="1.0" encoding="UTF-8"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><sz val="11"/><color rgb="FFFF0000"/><name val="Calibri"/></font></fonts><fills count="1"><fill><patternFill patternType="none"/></fill></fills><borders count="1"><border/></borders><cellStyleXfs count="1"><xf/></cellStyleXfs><cellXfs count="2"><xf xfId="0"/><xf xfId="0" fontId="1" applyFont="1"/></cellXfs></styleSheet>`
