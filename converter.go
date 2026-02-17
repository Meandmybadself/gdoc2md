package main

import (
	"fmt"
	"strings"

	docsv1 "google.golang.org/api/docs/v1"
)

// ConvertResult holds the markdown output and any image references found.
type ConvertResult struct {
	Markdown string
	Images   []ImageRef
}

// ImageRef represents an image to download.
type ImageRef struct {
	ObjectID   string
	ContentURI string
	Filename   string
}

// ConvertTab converts a single Google Docs tab to markdown.
// tabIndex is used to create globally unique image filenames across tabs.
func ConvertTab(tab *docsv1.Tab, tabTitle string, tabIndex int) ConvertResult {
	c := &converter{
		tab:      tab,
		tabIndex: tabIndex,
	}
	c.writeHeading(tabTitle, 1)
	if tab.DocumentTab != nil {
		c.convertBody(tab.DocumentTab.Body)
	}
	return ConvertResult{
		Markdown: c.buf.String(),
		Images:   c.images,
	}
}

type converter struct {
	tab        *docsv1.Tab
	tabIndex   int
	buf        strings.Builder
	images     []ImageRef
	imageCount int
	listState  listTracker
}

type listTracker struct {
	listID       string
	nestingLevel int64
	itemCounts   map[int64]int
}

func (c *converter) writeHeading(text string, level int) {
	c.buf.WriteString(strings.Repeat("#", level))
	c.buf.WriteString(" ")
	c.buf.WriteString(strings.TrimSpace(text))
	c.buf.WriteString("\n\n")
}

func (c *converter) convertBody(body *docsv1.Body) {
	if body == nil {
		return
	}
	for _, elem := range body.Content {
		c.convertStructuralElement(elem)
	}
}

func (c *converter) convertStructuralElement(elem *docsv1.StructuralElement) {
	switch {
	case elem.Paragraph != nil:
		c.convertParagraph(elem.Paragraph)
	case elem.Table != nil:
		c.convertTable(elem.Table)
	case elem.SectionBreak != nil:
		// ignore
	case elem.TableOfContents != nil:
		// ignore — we generate our own
	}
}

func (c *converter) convertParagraph(p *docsv1.Paragraph) {
	style := p.ParagraphStyle
	namedStyle := ""
	if style != nil {
		namedStyle = style.NamedStyleType
	}

	headingLevel := headingLevelFromStyle(namedStyle)

	// Handle bullet lists.
	if p.Bullet != nil {
		c.handleListItem(p, headingLevel)
		return
	}

	// Reset list state when we leave a list.
	c.listState = listTracker{}

	// Build the text content of this paragraph.
	text := c.renderParagraphElements(p.Elements)

	// Skip empty paragraphs.
	if strings.TrimSpace(text) == "" {
		c.buf.WriteString("\n")
		return
	}

	if headingLevel > 0 {
		c.writeHeading(text, headingLevel)
		return
	}

	// Normal paragraph.
	c.buf.WriteString(strings.TrimRight(text, "\n"))
	c.buf.WriteString("\n\n")
}

func (c *converter) handleListItem(p *docsv1.Paragraph, headingLevel int) {
	bullet := p.Bullet
	nestingLevel := bullet.NestingLevel
	listID := bullet.ListId

	// Determine if this is an ordered list.
	ordered := false
	if listID != "" && c.tab.DocumentTab != nil && c.tab.DocumentTab.Lists != nil {
		if list, ok := c.tab.DocumentTab.Lists[listID]; ok {
			if list.ListProperties != nil && int(nestingLevel) < len(list.ListProperties.NestingLevels) {
				nl := list.ListProperties.NestingLevels[nestingLevel]
				ordered = isOrderedGlyph(nl.GlyphType)
			}
		}
	}

	// Reset counters when switching to a different list.
	if c.listState.listID != listID {
		c.listState = listTracker{
			listID:     listID,
			itemCounts: make(map[int64]int),
		}
	}

	if c.listState.itemCounts == nil {
		c.listState.itemCounts = make(map[int64]int)
	}

	// Reset counts for deeper levels when nesting decreases.
	if nestingLevel < c.listState.nestingLevel {
		for k := range c.listState.itemCounts {
			if k > nestingLevel {
				delete(c.listState.itemCounts, k)
			}
		}
	}

	c.listState.nestingLevel = nestingLevel
	c.listState.itemCounts[nestingLevel]++

	indent := strings.Repeat("  ", int(nestingLevel))
	text := strings.TrimSpace(c.renderParagraphElements(p.Elements))

	if ordered {
		c.buf.WriteString(fmt.Sprintf("%s%d. %s\n", indent, c.listState.itemCounts[nestingLevel], text))
	} else {
		c.buf.WriteString(fmt.Sprintf("%s- %s\n", indent, text))
	}
}

func (c *converter) renderParagraphElements(elements []*docsv1.ParagraphElement) string {
	var sb strings.Builder
	for _, elem := range elements {
		switch {
		case elem.TextRun != nil:
			sb.WriteString(c.renderTextRun(elem.TextRun))
		case elem.InlineObjectElement != nil:
			sb.WriteString(c.renderInlineObject(elem.InlineObjectElement))
		case elem.HorizontalRule != nil:
			sb.WriteString("\n---\n")
		}
	}
	return sb.String()
}

func (c *converter) renderTextRun(tr *docsv1.TextRun) string {
	text := tr.Content
	if text == "\n" {
		return text
	}

	style := tr.TextStyle
	if style == nil {
		return text
	}

	// Detect monospace font -> inline code.
	if isMonospace(style) && strings.TrimSpace(text) != "" {
		return "`" + strings.TrimSpace(text) + "`"
	}

	// Trim trailing newline for formatting, re-add after.
	trailingNewline := strings.HasSuffix(text, "\n")
	text = strings.TrimRight(text, "\n")
	if text == "" {
		if trailingNewline {
			return "\n"
		}
		return ""
	}

	// Apply formatting. Bold/italic first, then strikethrough wraps outermost.
	if style.Bold && style.Italic {
		text = "***" + text + "***"
	} else if style.Bold {
		text = "**" + text + "**"
	} else if style.Italic {
		text = "*" + text + "*"
	}
	if style.Strikethrough {
		text = "~~" + text + "~~"
	}

	// Wrap in link if present.
	if style.Link != nil && style.Link.Url != "" {
		text = "[" + text + "](" + style.Link.Url + ")"
	}

	if trailingNewline {
		text += "\n"
	}
	return text
}

func (c *converter) renderInlineObject(elem *docsv1.InlineObjectElement) string {
	if c.tab.DocumentTab == nil || c.tab.DocumentTab.InlineObjects == nil {
		return ""
	}
	obj, ok := c.tab.DocumentTab.InlineObjects[elem.InlineObjectId]
	if !ok {
		return ""
	}
	if obj.InlineObjectProperties == nil || obj.InlineObjectProperties.EmbeddedObject == nil {
		return ""
	}
	embedded := obj.InlineObjectProperties.EmbeddedObject
	if embedded.ImageProperties == nil || embedded.ImageProperties.ContentUri == "" {
		return ""
	}

	c.imageCount++
	ext := guessImageExtension(embedded.ImageProperties.ContentUri)
	filename := fmt.Sprintf("tab%d_image_%03d%s", c.tabIndex, c.imageCount, ext)
	alt := embedded.Title
	if alt == "" {
		alt = embedded.Description
	}
	if alt == "" {
		alt = filename
	}

	c.images = append(c.images, ImageRef{
		ObjectID:   elem.InlineObjectId,
		ContentURI: embedded.ImageProperties.ContentUri,
		Filename:   filename,
	})

	return fmt.Sprintf("![%s](images/%s)", alt, filename)
}

func (c *converter) convertTable(table *docsv1.Table) {
	if table == nil || len(table.TableRows) == 0 {
		return
	}

	rows := make([][]string, len(table.TableRows))
	for i, row := range table.TableRows {
		cells := make([]string, len(row.TableCells))
		for j, cell := range row.TableCells {
			var cellText strings.Builder
			for _, elem := range cell.Content {
				if elem.Paragraph != nil {
					text := c.renderParagraphElements(elem.Paragraph.Elements)
					cellText.WriteString(strings.TrimSpace(text))
				}
			}
			cells[j] = strings.ReplaceAll(cellText.String(), "|", "\\|")
			cells[j] = strings.ReplaceAll(cells[j], "\n", " ")
		}
		rows[i] = cells
	}

	if len(rows) == 0 {
		return
	}

	// First row is the header.
	c.buf.WriteString("| " + strings.Join(rows[0], " | ") + " |\n")
	sep := make([]string, len(rows[0]))
	for i := range sep {
		sep[i] = "---"
	}
	c.buf.WriteString("| " + strings.Join(sep, " | ") + " |\n")
	for _, row := range rows[1:] {
		for len(row) < len(rows[0]) {
			row = append(row, "")
		}
		c.buf.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	c.buf.WriteString("\n")
}

func headingLevelFromStyle(style string) int {
	switch style {
	case "HEADING_1":
		return 1
	case "HEADING_2":
		return 2
	case "HEADING_3":
		return 3
	case "HEADING_4":
		return 4
	case "HEADING_5":
		return 5
	case "HEADING_6":
		return 6
	case "TITLE":
		return 1
	case "SUBTITLE":
		return 2
	default:
		return 0
	}
}

func isOrderedGlyph(glyphType string) bool {
	switch glyphType {
	case "DECIMAL", "ALPHA", "UPPER_ALPHA", "ROMAN", "UPPER_ROMAN", "ZERO_DECIMAL":
		return true
	default:
		return false
	}
}

func isMonospace(style *docsv1.TextStyle) bool {
	if style.WeightedFontFamily == nil {
		return false
	}
	family := strings.ToLower(style.WeightedFontFamily.FontFamily)
	switch family {
	case "courier new", "consolas", "roboto mono", "source code pro",
		"fira code", "jetbrains mono", "ubuntu mono", "ibm plex mono",
		"dejavu sans mono", "menlo", "monaco", "andale mono":
		return true
	default:
		return strings.Contains(family, "mono") || strings.Contains(family, "courier")
	}
}

func guessImageExtension(uri string) string {
	lower := strings.ToLower(uri)
	switch {
	case strings.Contains(lower, ".png"):
		return ".png"
	case strings.Contains(lower, ".gif"):
		return ".gif"
	case strings.Contains(lower, ".svg"):
		return ".svg"
	case strings.Contains(lower, ".webp"):
		return ".webp"
	default:
		return ".jpg"
	}
}
