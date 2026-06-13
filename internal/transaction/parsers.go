package transaction

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// extractIndices extracts the key byte indices.
// Regex `(\(\w{1}\[(\d{1,2})\],\s*16\))+` finds all matches; first int = row_index,
// rest = key_bytes_indices.
func extractIndices(ondemandJS string) (rowIndex int, keyByteIndices []int, err error) {
	matches := IndicesRE.FindAllStringSubmatch(ondemandJS, -1)
	if len(matches) == 0 {
		return 0, nil, errors.New("Couldn't get KEY_BYTE indices")
	}
	all := make([]int, 0, len(matches))
	for _, m := range matches {
		// m[2] is the captured digit group
		n, perr := strconv.Atoi(m[2])
		if perr != nil {
			return 0, nil, fmt.Errorf("parse index %q: %w", m[2], perr)
		}
		all = append(all, n)
	}
	return all[0], all[1:], nil
}

// extractKey selects <meta name="twitter-site-verification"> and returns content attr.
func extractKey(doc *goquery.Document) (string, error) {
	sel := doc.Find("meta[name='twitter-site-verification']").First()
	if sel.Length() == 0 {
		return "", errors.New("Couldn't get [twitter-site-verification] key from the page source")
	}
	content, ok := sel.Attr("content")
	if !ok {
		return "", errors.New("twitter-site-verification meta has no content attr")
	}
	return content, nil
}

// extractFrames selects [id^='loading-x-anim'].
func extractFrames(doc *goquery.Document) *goquery.Selection {
	return doc.Find("[id^='loading-x-anim']")
}

// extract2DArray walks the SVG path data into a 2D int array.
//
//	frames[key_bytes[5] % 4] -> children[0] -> children[1] -> attr 'd'
//	then [9:] sliced, split on 'C', each piece -> regex non-digits to space, split, ints.
//
// goquery's .Children() returns only element nodes, matching the intended traversal.
func extract2DArray(frames *goquery.Selection, keyBytes []byte) ([][]int, error) {
	if len(keyBytes) < 6 {
		return nil, errors.New("key_bytes too short for index 5")
	}
	idx := int(keyBytes[5]) % 4
	if frames.Length() <= idx {
		return nil, fmt.Errorf("frame index %d out of range (have %d)", idx, frames.Length())
	}
	frame := frames.Eq(idx)
	// children[0] -> children[1]
	first := frame.Children().First()
	if first.Length() == 0 {
		return nil, errors.New("no children in frame")
	}
	second := first.Children().Eq(1)
	if second.Length() == 0 {
		return nil, errors.New("no children[1] in inner element")
	}
	dAttr, ok := second.Attr("d")
	if !ok {
		return nil, errors.New("inner element missing 'd' attribute")
	}
	if len(dAttr) < 9 {
		return nil, fmt.Errorf("'d' attribute too short: %q", dAttr)
	}
	body := dAttr[9:]
	pieces := strings.Split(body, "C")
	out := make([][]int, 0, len(pieces))
	for _, p := range pieces {
		// Replace runs of non-digits with single space, split, parse ints.
		cleaned := strings.TrimSpace(NonDigitRE.ReplaceAllString(p, " "))
		if cleaned == "" {
			out = append(out, nil)
			continue
		}
		fields := strings.Fields(cleaned)
		row := make([]int, 0, len(fields))
		for _, f := range fields {
			n, err := strconv.Atoi(f)
			if err != nil {
				return nil, fmt.Errorf("parse svg int %q: %w", f, err)
			}
			row = append(row, n)
		}
		out = append(out, row)
	}
	return out, nil
}

// GetOndemandFileURL builds the ondemand.s.*.js URL from the home page HTML.
func GetOndemandFileURL(homeHTML string) (string, error) {
	idxMatch := OnDemandFileRE.FindStringSubmatch(homeHTML)
	if len(idxMatch) < 2 {
		return "", errors.New("ondemand file index not found in home page")
	}
	hashRE, err := regexp.Compile(fmt.Sprintf(OnDemandHashFormat, idxMatch[1]))
	if err != nil {
		return "", err
	}
	hashMatch := hashRE.FindStringSubmatch(homeHTML)
	if len(hashMatch) < 2 {
		return "", errors.New("ondemand file hash not found")
	}
	return strings.Replace(OnDemandFileURL, "{filename}", hashMatch[1], 1), nil
}
