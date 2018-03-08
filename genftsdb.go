// +build ignore

package main

import (
	"database/sql"
	"log"
	"os"
	"regexp"

	"bitbucket.org/oov/dgf"
	"github.com/lestrrat-go/ngram"
	_ "github.com/mattn/go-sqlite3"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/oov/pnx/adapter"
)

var reRemoveSpaces = regexp.MustCompile(`\s+`)

func createDB(db *sql.DB) error {
	s := `CREATE TABLE aktnb (id INTEGER PRIMARY KEY AUTOINCREMENT, no INTEGER)`
	_, err := db.Exec(s)
	if err != nil {
		return err
	}

	s = `CREATE VIRTUAL TABLE aktnb_fts USING fts4(words TEXT)`
	_, err = db.Exec(s)
	if err != nil {
		return err
	}

	return nil
}

func createFTSDB(df *dgf.Dgf, db *sql.DB, n, start, end int) error {
	tx, err := db.Begin()
	if err != nil {
		log.Fatalln(err)
	}

	insAktnb, err := tx.Prepare(`INSERT INTO aktnb(no) VALUES (?)`)
	if err != nil {
		log.Fatalln(err)
	}

	insAktnbFTS, err := tx.Prepare(`INSERT INTO aktnb_fts(words) VALUES (?)`)
	if err != nil {
		log.Fatalln(err)
	}

	buf := make([]byte, 0, 10240)
	for i := start; i < end; i++ {
		song, err := df.GetSong(i)
		if err != nil {
			return err
		}
		q, err := song.GetDocument()
		if err != nil {
			return err
		}
		q.Find(`script, head, form, #header, h2, hr, #form, #guidebar, #footer, div[style*="!important"], div[style*="z-index:9999"]`).Remove()
		text := q.Find("html").Text()
		text = reRemoveSpaces.ReplaceAllString(text, " ")
		if text == " \u4F55\u3082\u898B\u3064\u304B\u3089\u306A\u3044\u304A\uFF08 \uFF3E\u03C9\uFF3E\uFF09 " {
			continue
		}
		tkz := ngram.NewTokenize(n, text)
		tokens := tkz.Tokens()
		for {
			tk := tokens[len(tokens)-1]
			if tk.Start() == tk.End()-1 {
				break
			}
			tokens = append(tokens, tkz.NewToken(tk.Start()+1, tk.End()))
		}
		for _, tk := range tokens {
			buf = append(buf, tk.String()...)
			buf = append(buf, ' ')
		}
		_, err = insAktnb.Exec(i)
		if err != nil {
			return err
		}
		_, err = insAktnbFTS.Exec(string(buf))
		if err != nil {
			return err
		}
		buf = buf[:0]
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func main() {
	os.Remove("fts.db")
	ftsdb, err := sql.Open("sqlite3", "fts.db")
	if err != nil {
		log.Fatal(err)
	}
	defer ftsdb.Close()

	err = createDB(ftsdb)
	if err != nil {
		log.Fatalln(err)
	}

	db, err := leveldb.OpenFile("sdc.db", &opt.Options{})
	if err != nil {
		log.Fatalln("Cannot open db:", err)
	}
	df := dgf.New(adapter.NewLevelDBAdapter(db))
	defer df.Close()

	maxID, err := df.GetLocalMaxId()
	if err != nil {
		log.Fatalln(err)
	}

	const blockSize = 500
	for i := 1; i <= maxID; i += blockSize {
		end := i + blockSize
		if end > maxID {
			end = maxID + 1
		}
		err = createFTSDB(df, ftsdb, 3, i, end)
		if err != nil {
			log.Fatalln(i, err)
		}
		log.Println(i, "～", end-1)
	}
}
