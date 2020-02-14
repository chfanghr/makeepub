package main

import (
	"bufio"
	"fmt"
	"github.com/bmaupin/go-epub"
	"github.com/chfanghr/chinese_number"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
)

type paraHead struct {
	id    int
	title string
}

func (h paraHead) String() string {
	if h.id != -1 {
		return fmt.Sprintf("%d %s", h.id, h.title)
	}
	return h.title
}

type para struct {
	paraHead
	lines []string
}

type paras []para

func (p paras) Len() int {
	return len(p)
}

func (p paras) Less(i, j int) bool {
	return p[i].id < p[j].id
}

func (p paras) Swap(i, j int) {
	tmp := p[j]
	p[j] = p[i]
	p[i] = tmp
}

type novelHead struct {
	title  string
	author string
}

type novel struct {
	novelHead
	paras paras
}

func (n novel) toEpub() (*epub.Epub, error) {
	sort.Sort(n.paras)

	e := epub.NewEpub(n.title)

	e.SetAuthor(n.author)

	for _, para := range n.paras {
		sectionBody := "<h1>" + para.paraHead.String() + "</h1>\n<p></p>\n"
		for _, line := range para.lines {
			sectionBody += "<p>" + line + "</p>\n"
		}
		if _, err := e.AddSection(sectionBody, para.paraHead.String(), "", ""); err != nil {
			return nil, err
		}
	}

	return e, nil
}

func isArabic(ch rune) bool {
	return '0' <= ch && ch <= '9'
}

func isDigit(ch rune) bool {
	if '0' <= ch && ch <= '9' {
		return true
	}

	switch ch {
	case '零', '一', '二', '三', '四', '五',
		'六', '七', '八', '九', '十',
		'百', '千', '万':
		return true
	}

	return false
}

func parseLine(line string, n *novel, lastStat bool) (isUnknown bool) {
	if len(line) == 0 {
		return true
	}

	runeLine := []rune(line)

	switch runeLine[0] {
	case '《': // novel header
		hasRightAngleQuotationMark := false
		var title, author []rune
		i := 1

		for ; i < len(runeLine); i++ {
			if runeLine[i] == '》' {
				hasRightAngleQuotationMark = true
				break
			}
			title = append(title, runeLine[i])
		}
		if !hasRightAngleQuotationMark {
			log.Println("novel title doesn't have matched angle quotation marks")
			return true
		}

		if i <= len(runeLine)-7 && string(runeLine[i+1:i+7]) == " - 作者：" { // parse author
			log.Println("novel doesn't have an author")
			author = runeLine[i+7:]
		}

		n.novelHead = novelHead{
			title:  string(title),
			author: string(author),
		}
	case ' ', '　': // parse content of paragraph
		if lastStat {
			log.Println("waiting for next valid header....")
			return true
		}
		i := 0
		for ; i < len(runeLine) && (runeLine[i] == ' ' || runeLine[i] == '\t' || runeLine[i] == '　'); i++ {
		}
		if i == len(runeLine) {
			log.Println("empty line in content")
			return false
		}
		n.paras[len(n.paras)-1].lines = append(n.paras[len(n.paras)-1].lines, string(runeLine[i:]))
	case '第': // parse title of paragraph
		i := 1

		var runeId []rune

		for ; i < len(runeLine); i++ {
			if isDigit(runeLine[i]) {
				runeId = append(runeId, runeLine[i])
			} else {
				break
			}
		}

		if len(runeId) == 0 || runeLine[i] != '章' {
			log.Println("invalid title of paragraph")
			return true
		}

		var err error
		var id int

		if isArabic(runeId[0]) {
			id, err = strconv.Atoi(string(runeId))
			if err != nil {
				log.Println("cannot parse id of paragraph: ", err)
				return true
			}
		} else {
			id, err = chinese_number.ToArabicNumber(string(runeId))
			if err != nil {
				var altId []int
				for _, r := range runeId {
					num, err := chinese_number.ParseChineseNumberCharacter(r)
					if err != nil {
						log.Println("cannot parse id of paragraph: ", err)
						return true
					}
					altId = append(altId, num.GetValue())
				}
				factor := 1
				for i = len(altId) - 1; i >= 0; i-- {
					id += altId[i] * factor
					factor *= 10
				}
			}
		}

		n.paras = append(n.paras, para{
			paraHead: paraHead{
				id:    id,
				title: string(runeLine[i+2:]),
			},
			lines: nil,
		})
	default:
		if regexp.MustCompile("番外：(.*?)").MatchString(line) {
			n.paras = append(n.paras, para{
				paraHead: paraHead{
					id:    -1,
					title: line,
				},
				lines: nil,
			})
			return false
		}
		return true
	}

	return false
}

func safeClose(closer io.Closer) {
	if err := closer.Close(); err != nil {
		log.Panicf("error: %v", err)
	}
}

func main() {
	file, err := os.Open(os.Args[1])
	if err != nil {
		log.Panicf("error: %v", err)
	}
	defer safeClose(file)

	scanner := bufio.NewScanner(file)
	novel := novel{}
	lineId := uint64(1)

	log.Printf("processing %v...", os.Args[1])
	log.Println("parsing...")

	lastStat := false

	for scanner.Scan() {
		line := scanner.Text()
		if lastStat = parseLine(line, &novel, lastStat); lastStat {
			log.Printf("unknown line %d: %s", lineId, line)
		}
		lineId++
	}

	log.Println("parse: done")

	log.Println("converting to epub...")

	e, err := novel.toEpub()

	if err != nil {
		log.Panicf("error: %v", err)
	}

	log.Println("convert: done")

	if novel.title == "" {
		log.Println("novel doesn't have a title, use filename instead")
		novel.title = os.Args[1]
	}

	log.Printf("writing %s to disk...", novel.title+".epub")

	if err = e.Write(novel.title + ".epub"); err != nil {
		log.Panicf("error: %v", err)
	}

	log.Println("write: done")
	log.Println("process: done")
}
