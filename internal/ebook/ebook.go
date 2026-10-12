package ebook

import (
	"bytes"
	"fmt"
	"strings"
)

// SniffLen is how many header bytes a caller should read before
// calling Validate. All the signatures checked here live well within
// this many bytes - acsm's XML namespace marker is the one that needs
// the most room, since it doesn't show up until a bit into the file.
const SniffLen = 256

// allowedExtensions is the set of ebook formats calibredb add is
// expected to handle, keyed by lowercase extension (without the dot).
var allowedExtensions = map[string]bool{
	"epub":  true,
	"acsm":  true,
	"mobi":  true,
	"azw":   true,
	"azw3":  true,
	"azw4":  true,
	"fb2":   true,
	"pdf":   true,
	"djvu":  true,
	"cbz":   true,
	"cbr":   true,
	"txt":   true,
	"rtf":   true,
	"docx":  true,
	"odt":   true,
	"htmlz": true,
	"html":  true,
	"lit":   true,
	"lrf":   true,
}

// magicCheckers holds format-specific magic-byte validators for the
// formats where a cheap, reliable signature exists. Formats with no
// entry here (txt, rtf's edge cases, html, etc.) are trusted on
// extension alone.
var magicCheckers = map[string]func([]byte) bool{
	"epub": isEPUB,
	"pdf":  isPDF,
	"mobi": isMobiOrAzw,
	"azw":  isMobiOrAzw,
	"azw3": isMobiOrAzw,
	"azw4": isMobiOrAzw,
	"cbz":  isZip, // comic book zip archive
	"docx": isZip, // OOXML is a zip container
	"odt":  isZip, // ODF is a zip container
	"acsm": isACSM,
}

func isZip(header []byte) bool {
	return bytes.HasPrefix(header, []byte("PK\x03\x04"))
}

func isEPUB(header []byte) bool {
	// EPUB is a zip archive whose first entry is a stored (uncompressed)
	// "mimetype" file. Checking for the zip signature catches the
	// overwhelming majority of spoofing attempts without needing a full
	// zip-directory parse.
	return isZip(header)
}

func isPDF(header []byte) bool {
	return bytes.HasPrefix(header, []byte("%PDF-"))
}

func isACSM(header []byte) bool {
	// ACSM ("Adobe Content Server Message") files are small XML
	// "fulfillment token" documents, not the book itself - they just
	// point Adobe Digital Editions (or calibre's DeACSM plugin) at the
	// real download. Real-world samples have no XML declaration; they
	// go straight into the root element, e.g.:
	//   <fulfillmentToken fulfillmentType="loan" auth="user"
	//     xmlns="http://ns.adobe.com/adept">
	// So check for an opening tag (allowing a leading UTF-8
	// BOM/whitespace) plus both the root element name and Adobe's
	// "adept" fulfillment namespace, which together are distinctive
	// enough to rule out arbitrary XML/text files.
	trimmed := bytes.TrimLeft(header, "\xef\xbb\xbf \t\r\n")
	if !bytes.HasPrefix(trimmed, []byte("<")) {
		return false
	}
	lower := bytes.ToLower(trimmed)
	return bytes.Contains(lower, []byte("fulfillmenttoken")) && bytes.Contains(lower, []byte("ns.adobe.com/adept"))
}

func isMobiOrAzw(header []byte) bool {
	// PalmDOC/MOBI container: the magic cookie "BOOKMOBI" or "TPZ3" sits
	// at offset 60 in the file header.
	if len(header) < 68 {
		return false
	}
	cookie := header[60:68]
	return bytes.Equal(cookie, []byte("BOOKMOBI")) || bytes.HasPrefix(cookie, []byte("TPZ"))
}

// extOf returns the lowercase extension of filename, without the dot,
// or "" if there isn't one.
func extOf(filename string) string {
	i := strings.LastIndexByte(filename, '.')
	if i < 0 || i == len(filename)-1 {
		return ""
	}
	return strings.ToLower(filename[i+1:])
}

// Validate checks that filename's extension is an accepted ebook
// format and, where a cheap magic-byte signature exists for that
// format, that header actually matches it. header may be shorter than
// SniffLen if the file itself is tiny.
func Validate(filename string, header []byte) error {
	ext := extOf(filename)
	if ext == "" {
		return fmt.Errorf("file has no extension")
	}
	if !allowedExtensions[ext] {
		return fmt.Errorf("unsupported ebook format %q", ext)
	}
	if check, ok := magicCheckers[ext]; ok {
		if !check(header) {
			return fmt.Errorf("file content does not match expected format for .%s", ext)
		}
	}
	return nil
}
