package main

import (
	"database/sql"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"bitbucket.org/oov/dgf"
	"github.com/PuerkitoBio/goquery"
	"github.com/labstack/echo"
	"github.com/lestrrat-go/ngram"
	_ "github.com/mattn/go-sqlite3"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"golang.org/x/text/unicode/norm"

	"github.com/oov/pnx/adapter"
)

const PAGESIZE = 20

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

type searchVars struct {
	Query  string
	Count  int
	Page   int
	Offset int
	Last   int
	Songs  []*dgf.Song
}

func (v *searchVars) NextLink() template.URL {
	return template.URL("/?q=" + template.URLQueryEscaper(v.Query) + "&p=" + template.URLQueryEscaper(v.Page+1))
}

func (v *searchVars) PrevLink() template.URL {
	return template.URL("/?q=" + template.URLQueryEscaper(v.Query) + "&p=" + template.URLQueryEscaper(v.Page-1))
}

func (v *searchVars) HasNext() bool {
	return v.Count > PAGESIZE && v.Last < v.Count
}

func (v *searchVars) HasPrev() bool {
	return v.Count > PAGESIZE && v.Offset > 1
}

func (v *searchVars) LastLink() template.URL {
	return template.URL("/?q=" + template.URLQueryEscaper(v.Query) + "&p=" + template.URLQueryEscaper((v.Count+PAGESIZE-1)/PAGESIZE-1))
}

func (v *searchVars) FirstLink() template.URL {
	return template.URL("/?q=" + template.URLQueryEscaper(v.Query) + "&p=0")
}

func (v *searchVars) HasLast() bool {
	return v.Count > PAGESIZE && v.Last < v.Count
}

func (v *searchVars) HasFirst() bool {
	return v.Count > PAGESIZE && v.Offset > 1
}

const searchTemplate = `<!DOCTYPE html>
<style>
.pnx-pager { list-style-type: none; }
.pnx-pager li { display: inline; }
.pnx-dead { color: #ccc; cursor: default; }
.pnx-songlist dt { font-weight: bold; }
</style>
<form action="/" method="get">
	<input type="text" name="q" autocomplete="off" value="{{.Query}}"><input type="hidden" name="p" value="0"><input type="submit" value="検索">
</form>
{{if .Count}}
	<p>全 {{.Count}} 件中 {{.Offset}}～{{.Last}}件目</p>
	<ul class="pnx-pager">
		<li><a {{if .HasFirst}}class="pnx-live" href="{{.FirstLink}}"{{else}}class="pnx-dead" href="javascript:;"{{end}}>&laquo; 最初へ</a></li>
		<li><a {{if .HasPrev}}class="pnx-live" href="{{.PrevLink}}"{{else}}class="pnx-dead" href="javascript:;"{{end}}>&laquo; 前へ</a></li>
		<li><a {{if .HasNext}}class="pnx-live" href="{{.NextLink}}"{{else}}class="pnx-dead" href="javascript:;"{{end}}>次へ &raquo;</a></li>
		<li><a {{if .HasLast}}class="pnx-live" href="{{.LastLink}}"{{else}}class="pnx-dead" href="javascript:;"{{end}}>最後へ &raquo;</a></li>
	</ul>
	<ul class="pnx-songlist">
		{{range .Songs}}
			<li>
				<a href="/{{.GetIndex}}/">No.{{.GetIndex}}</a>
				<dl>
					{{range .Entries}}
						<dt>{{.Title}}</dt>
						{{if .HasLyric}}
						  <dd>{{ShowLyricIfExists .}}</dd>
						{{end}}
					{{end}}
				</dl>
			</li>
		{{end}}
	</ul>
	<ul class="pnx-pager">
	<li><a {{if .HasFirst}}class="pnx-live" href="{{.FirstLink}}"{{else}}class="pnx-dead" href="javascript:;"{{end}}>&laquo; 最初へ</a></li>
	<li><a {{if .HasPrev}}class="pnx-live" href="{{.PrevLink}}"{{else}}class="pnx-dead" href="javascript:;"{{end}}>&laquo; 前へ</a></li>
	<li><a {{if .HasNext}}class="pnx-live" href="{{.NextLink}}"{{else}}class="pnx-dead" href="javascript:;"{{end}}>次へ &raquo;</a></li>
	<li><a {{if .HasLast}}class="pnx-live" href="{{.LastLink}}"{{else}}class="pnx-dead" href="javascript:;"{{end}}>最後へ &raquo;</a></li>
</ul>
{{else}}
	<p>一致する項目が見つかりませんでした。</p>
{{end}}
`

func main() {
	db, err := leveldb.OpenFile("sdc.db", &opt.Options{})
	if err != nil {
		log.Fatalln("Cannot open db:", err)
	}
	df := dgf.New(adapter.NewLevelDBAdapter(db))
	defer df.Close()

	ftsdb, err := sql.Open("sqlite3", "fts.db")
	if err != nil {
		log.Fatal(err)
	}
	defer ftsdb.Close()

	countStmt, err := ftsdb.Prepare("SELECT COUNT(*) FROM aktnb_fts WHERE words MATCH ?")
	if err != nil {
		log.Fatal(err)
	}

	queryStmt, err := ftsdb.Prepare("SELECT no FROM aktnb_fts INNER JOIN aktnb ON aktnb_fts.docid = aktnb.id WHERE words MATCH ? ORDER BY id DESC LIMIT ?, ?")
	if err != nil {
		log.Fatal(err)
	}

	countAllStmt, err := ftsdb.Prepare("SELECT COUNT(*) FROM aktnb")
	if err != nil {
		log.Fatal(err)
	}

	queryAllStmt, err := ftsdb.Prepare("SELECT no FROM aktnb ORDER BY id DESC LIMIT ?, ?")
	if err != nil {
		log.Fatal(err)
	}

	e := echo.New()
	e.HideBanner = true
	e.Renderer = &Template{
		templates: template.Must(template.New("search").Funcs(template.FuncMap{
			"ShowLyricIfExists": func(e *dgf.SongEntry) template.HTML {
				ly, err := e.GetLyric()
				if err != nil {
					return ""
				}
				return template.HTML(strings.Replace(strings.TrimSpace(template.HTMLEscapeString(ly.Body)), "\n", "<br>", -1))
			},
		}).Parse(searchTemplate)),
	}
	e.GET("/:id/", func(c echo.Context) error {
		idx, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.Error(err)
			return c.String(http.StatusNotFound, "not found")
		}
		song, err := df.GetSong(idx)
		if err != nil {
			c.Error(err)
			return c.String(http.StatusNotFound, "not found")
		}
		q, err := song.GetDocument()
		if err != nil {
			c.Error(err)
			return c.String(http.StatusNotFound, "not found")
		}
		q.Find(`script, head, form, #header, h2, hr, #form, #guidebar, #footer, div[style*="!important"], div[style*="z-index:9999"]`).Remove()
		q.Find(`a[href*="songlist.php?key="], a[href^="?key="]`).Each(func(i int, s *goquery.Selection) {
			attr, ok := s.Attr("href")
			if !ok {
				return
			}
			start := strings.Index(attr, "?key=no%3A")
			if start == -1 {
				return
			}
			start += 10
			end := start
			for ; end < len(attr); end++ {
				c := attr[end] - '0'
				if c < 0 || c > 9 {
					break
				}
			}
			s.SetAttr("href", "/"+attr[start:end]+"/")
			s.SetAttr("data-orig-href", attr)
		})
		htmlElem := q.Find("html")
		htmlElem.PrependHtml(`<meta charset="UTF-8">`)
		s, err := htmlElem.Html()
		if err != nil {
			c.Error(err)
			return c.String(http.StatusNotFound, "not found")
		}
		return c.HTML(http.StatusOK, "<!DOCTYPE html>"+s)
	})

	e.GET("/", func(c echo.Context) error {
		var v searchVars
		q := c.QueryParam("q")
		if len(q) > 64 {
			return c.Redirect(http.StatusFound, "/")
		}
		v.Query = q

		page, _ := strconv.Atoi(c.QueryParam("p"))
		var rows *sql.Rows
		if q == "" {
			err := countAllStmt.QueryRow().Scan(&v.Count)
			if err != nil {
				c.Error(err)
				return c.String(http.StatusNotFound, "not found")
			}
			if v.Count == 0 {
				return c.Render(http.StatusOK, "search", &v)
			}
			if page < 0 || page > v.Count/PAGESIZE {
				page = 0
			}
			rows, err = queryAllStmt.Query(PAGESIZE*page, PAGESIZE)
		} else {
			q = norm.NFKC.String(strings.TrimSpace(q))

			var buf []byte
			if len([]rune(q)) < 3 {
				buf = append(buf, q...)
				buf = append(buf, '*')
			} else {
				tkz := ngram.NewTokenize(3, q)
				tokens := tkz.Tokens()
				for _, tk := range tokens {
					buf = append(buf, tk.String()...)
					buf = append(buf, ' ')
				}
			}
			qq := string(buf)
			err := countStmt.QueryRow(qq).Scan(&v.Count)
			if err != nil {
				c.Error(err)
				return c.String(http.StatusNotFound, "not found")
			}
			if v.Count == 0 {
				return c.Render(http.StatusOK, "search", &v)
			}

			if page < 0 || page > v.Count/PAGESIZE {
				page = 0
			}
			rows, err = queryStmt.Query(qq, PAGESIZE*page, PAGESIZE)
		}
		if err != nil {
			c.Error(err)
			return c.String(http.StatusNotFound, "not found")
		}
		defer rows.Close()

		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				c.Error(err)
				return c.String(http.StatusNotFound, "not found")
			}
			ids = append(ids, id)
		}
		v.Page = page
		v.Offset = page*PAGESIZE + 1
		v.Last = v.Offset + len(ids) - 1

		for _, id := range ids {
			song, err := df.GetSong(id)
			if err != nil {
				c.Error(err)
				return c.String(http.StatusNotFound, "not found")
			}
			v.Songs = append(v.Songs, song)
		}

		return c.Render(http.StatusOK, "search", &v)
	})
	e.Logger.Fatal(e.Start("localhost:51123"))
}
