package feedparser

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ParsedFeed is the normalized result of parsing an RSS or Atom feed.
type ParsedFeed struct {
	Title       string
	Description string
	SiteURL     string
	Entries     []ParsedEntry
}

// ParsedEntry is a single item from a parsed feed.
type ParsedEntry struct {
	GUID        string
	Title       string
	Link        string
	Summary     string
	PublishedAt *time.Time
}

// FetchAndParse downloads a feed URL and parses it.
func FetchAndParse(url string) (*ParsedFeed, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return Parse(body)
}

// Parse takes raw XML bytes and parses either RSS 2.0 or Atom.
func Parse(data []byte) (*ParsedFeed, error) {
	// Try RSS first
	pf, err := parseRSS(data)
	if err == nil && pf != nil {
		return pf, nil
	}
	// Try Atom
	pf, err = parseAtom(data)
	if err == nil && pf != nil {
		return pf, nil
	}
	return nil, fmt.Errorf("could not parse feed as RSS or Atom")
}

// --- RSS 2.0 ---

type rssXML struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Description string    `xml:"description"`
	Link        string    `xml:"link"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
}

func parseRSS(data []byte) (*ParsedFeed, error) {
	var r rssXML
	if err := xml.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	pf := &ParsedFeed{
		Title:       r.Channel.Title,
		Description: r.Channel.Description,
		SiteURL:     r.Channel.Link,
	}
	for _, item := range r.Channel.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		pe := ParsedEntry{
			GUID:    guid,
			Title:   item.Title,
			Link:    item.Link,
			Summary: cleanHTML(item.Description),
		}
		if item.PubDate != "" {
			t := parseDate(item.PubDate)
			if t != nil {
				pe.PublishedAt = t
			}
		}
		pf.Entries = append(pf.Entries, pe)
	}
	return pf, nil
}

// --- Atom 1.0 ---

type atomXML struct {
	XMLName  xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle"`
	Links    []atomLink  `xml:"link"`
	Entries  []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Links     []atomLink `xml:"link"`
	ID        string     `xml:"id"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
}

func parseAtom(data []byte) (*ParsedFeed, error) {
	var a atomXML
	if err := xml.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	pf := &ParsedFeed{
		Title:       a.Title,
		Description: a.Subtitle,
	}
	for _, l := range a.Links {
		if l.Rel == "" || l.Rel == "alternate" {
			pf.SiteURL = l.Href
			break
		}
	}
	for _, entry := range a.Entries {
		guid := entry.ID
		link := ""
		for _, l := range entry.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				link = l.Href
				break
			}
		}
		summary := entry.Summary
		if summary == "" {
			summary = entry.Content
		}
		pe := ParsedEntry{
			GUID:    guid,
			Title:   entry.Title,
			Link:    link,
			Summary: cleanHTML(summary),
		}
		dateStr := entry.Published
		if dateStr == "" {
			dateStr = entry.Updated
		}
		if dateStr != "" {
			t := parseDate(dateStr)
			if t != nil {
				pe.PublishedAt = t
			}
		}
		pf.Entries = append(pf.Entries, pe)
	}
	return pf, nil
}

// --- Helpers ---

func parseDate(s string) *time.Time {
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 2 Jan 2006 15:04:05 MST",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, strings.TrimSpace(s))
		if err == nil {
			return &t
		}
	}
	return nil
}

func cleanHTML(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	result := strings.TrimSpace(out.String())
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#39;", "'")
	if len(result) > 500 {
		result = result[:500] + "..."
	}
	return result
}
