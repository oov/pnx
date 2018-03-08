package main

import (
	"database/sql"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"

	"bitbucket.org/oov/dgf"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/lestrrat-go/ngram"
	_ "github.com/mattn/go-sqlite3"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/oov/pnx/adapter"
)

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

type searchVars struct {
	Query  string
	Count  int
	Offset int
	Last   int
	Songs  []*dgf.Song
}

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

	e := echo.New()
	e.Renderer = &Template{
		templates: template.Must(template.New("search").Parse(`<!DOCTYPE html>
<form action="/search" method="get">
  <input type="text" name="q" autocomplete="off" value="{{.Query}}">
  <input type="submit" value="検索">
</form>
{{if .Query}}
  {{if .Count}}
    <p>全 {{.Count}} 件中 {{.Offset}}～{{.Last}}件目</p>
  	<ul>
  	{{range .Songs}}
  	  <li>
  		  <a href="/{{.GetIndex}}/">No.{{.GetIndex}}</a>
  			<dl>
  			  {{range .Entries}}
  				  <dt>{{.Title}}</dt>
  				{{end}}
  			</dl>
  		</li>
  	{{end}}
  	</ul>
  {{else}}
    <p>一致する項目が見つかりませんでした。</p>
  {{end}}
{{else}}
  <p>2013年7月頃に収集したデータを元に部分的に再現したデータベースです。</p>
	<p>記事個別ページ内のリンクは修正していないので、リンク切れが多数あります。</p>
{{end}}
`)),
	}
	e.Use(middleware.Logger())
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
		return c.Render(http.StatusOK, "search", &searchVars{})
	})
	e.GET("/search", func(c echo.Context) error {
		var v searchVars
		q := c.QueryParam("q")
		if q == "" {
			return c.Redirect(http.StatusFound, "/")
		}
		if len(q) > 64 {
			return c.Render(http.StatusOK, "search", &v)
		}
		v.Query = q

		tkz := ngram.NewTokenize(3, q)
		tokens := tkz.Tokens()
		var buf []byte
		for _, tk := range tokens {
			buf = append(buf, tk.String()...)
			buf = append(buf, ' ')
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

		const PAGESIZE = 20
		page, _ := strconv.Atoi(c.QueryParam("p"))
		if page < 0 || page > v.Count/PAGESIZE {
			page = 0
		}
		rows, err := queryStmt.Query(qq, PAGESIZE*page, PAGESIZE)
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
	e.Logger.Fatal(e.Start(":1323"))
}
