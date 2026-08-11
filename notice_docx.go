package main

import (
	"archive/zip"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var documentBreakRE = regexp.MustCompile(`(?is)<\s*(?:br\b[^>]*|/p\s*|/div\s*|/li\s*|/tr\s*|/h[1-6]\s*)[^>]*>`)

func writeNoticeDocx(archivePath string, item Opportunity, body string) (string, error) {
	path := noticeDocxPath(archivePath, item)
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	defer writer.Close()
	if err := writeDocxPart(writer, "[Content_Types].xml", docxContentTypes); err != nil {
		return "", err
	}
	if err := writeDocxPart(writer, "_rels/.rels", docxRelationships); err != nil {
		return "", err
	}
	if err := writeDocxPart(writer, "word/document.xml", buildDocumentXML(item, body)); err != nil {
		return "", err
	}
	return path, nil
}

func noticeDocxPath(archivePath string, item Opportunity) string {
	name := trimFileName(sanitizeFileName(item.Title), 110)
	if name == "" {
		name = "公告"
	}
	return filepath.Join(archivePath, name+".docx")
}

func writeDocxPart(writer *zip.Writer, name string, content string) error {
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = entry.Write([]byte(content))
	return err
}

func buildDocumentXML(item Opportunity, body string) string {
	paragraphs := []string{
		wordParagraph(item.Title, true),
		wordParagraph("来源："+item.SourceSite, false),
		wordParagraph("栏目："+item.CategoryName, false),
		wordParagraph("发布时间："+item.PublishTime, false),
		wordParagraph("公告编号："+item.TenderNo, false),
		wordParagraph("原文地址："+item.SourceURL, false),
	}
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paragraphs = append(paragraphs, wordParagraph(line, false))
		}
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + strings.Join(paragraphs, "") + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`
}

func wordParagraph(value string, bold bool) string {
	runProperties := `<w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Microsoft YaHei"/><w:sz w:val="22"/></w:rPr>`
	if bold {
		runProperties = `<w:rPr><w:b/><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Microsoft YaHei"/><w:sz w:val="30"/></w:rPr>`
	}
	return `<w:p><w:pPr><w:spacing w:after="120"/></w:pPr><w:r>` + runProperties + `<w:t xml:space="preserve">` + html.EscapeString(value) + `</w:t></w:r></w:p>`
}

func documentTextFromHTML(raw string) string {
	withBreaks := documentBreakRE.ReplaceAllString(raw, "\n")
	return strings.TrimSpace(stripTags(withBreaks))
}

const docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`
const docxRelationships = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`
