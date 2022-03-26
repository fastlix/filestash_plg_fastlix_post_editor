package plg_fastlix_post_editor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/mickael-kerjean/filestash/server/common"
)

type PostEditor struct {
	params map[string]string
	db     *sql.DB
}

func init() {
	Backend.Register("post_editor", PostEditor{})
}

func (e PostEditor) Init(params map[string]string, app *App) (IBackend, error) {
	if params["host"] == "" {
		params["host"] = "127.0.0.1"
	}
	if params["port"] == "" {
		params["port"] = "3306"
	}

	db, err := sql.Open(
		"mysql",
		fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/",
			params["username"],
			params["password"],
			params["host"],
			params["port"],
		),
	)
	if err != nil {
		return nil, err
	}
	return PostEditor{
		params: params,
		db:     db,
	}, nil
}

func (e PostEditor) LoginForm() Form {
	return Form{
		Elmnts: []FormElement{
			FormElement{
				Name:  "type",
				Type:  "hidden",
				Value: "mysql",
			},
			FormElement{
				Name:        "host",
				Type:        "text",
				Placeholder: "Host",
			},
			FormElement{
				Name:        "username",
				Type:        "text",
				Placeholder: "Username",
			},
			FormElement{
				Name:        "password",
				Type:        "password",
				Placeholder: "Password",
			},
			FormElement{
				Name:        "port",
				Type:        "number",
				Placeholder: "Port",
			},
		},
	}
}

func (e PostEditor) Ls(path string) (files []os.FileInfo, err error) {
	defer e.db.Close()

	p, err := parsePostPath(path)
	if err != nil {
		return
	}

	files = make([]os.FileInfo, 0)

	if p.lang == "" { // first level folder = a list all languages
		var rows *sql.Rows
		rows, err = e.db.Query("SELECT distinct lang from Posts")
		if err != nil {
			return
		}

		for rows.Next() {
			var lang string

			if err = rows.Scan(&lang); err != nil {
				return
			}

			files = append(files, File{
				FName: lang,
				FType: "directory",
			})
		}
	} else { // second level folder = a list of all posts for language
		var rows *sql.Rows
		rows, err = e.db.Query("SELECT slug, createdAt, updatedAt FROM Posts WHERE lang = ?", path)
		if err != nil {
			return
		}

		for rows.Next() {
			var name string
			var createAt string
			var rawCreateAt sql.RawBytes

			if err = rows.Scan(&name, &rawCreateAt); err != nil {
				return
			}

			createAt = string(rawCreateAt)

			files = append(files, File{
				FName: string(name) + ".md",
				FType: "file",
				FSize: -1,
				FTime: func() int64 {
					t, err := time.Parse("2006-01-02", fmt.Sprintf("%s", createAt))
					if err != nil {
						return 0
					}
					return t.Unix()
				}(),
			})
		}
	}
	return
}

func (e PostEditor) Cat(path string) (io.ReadCloser, error) {
	defer e.db.Close()

	p, err := parsePostPath(path)

	if p.lang == "" || p.slug == "" {
		return nil, fmt.Errorf("invalid post path")
	}

	var forms []FormElement

	b, err := Form{Elmnts: forms}.MarshalJSON()
	if err != nil {
		return nil, err
	}

	return NewReadCloserFromBytes(b), nil
}

func (e PostEditor) Mkdir(path string) error {
	defer e.db.Close()
	return ErrNotAllowed
}

func (e PostEditor) Rm(path string) error {
	defer e.db.Close()

	p, err := parsePostPath(path)

	if p.slug == "" {
		return fmt.Errorf("invalid path")
	}

	_, err = e.db.Exec("DELETE FROM Posts WHERE slug = ?", p.slug)
	return err
}

func (e PostEditor) Mv(from string, to string) error {
	defer e.db.Close()
	return ErrNotValid
}

func (e PostEditor) Touch(path string) (err error) {
	defer e.db.Close()

	p, err := parsePostPath(path)
	if err != nil {
		return
	}

	if p.lang == "" || p.slug == "" {
		err = fmt.Errorf("invalid path")
		return
	}

	_, err = e.db.Exec(
		"INSERT INTO Posts(lang, slug, createdAt, updatedAt) VALUES(?, ?, ?, ?)",
		p.lang,
		p.slug,
		time.Now().Format("2006/01/02 15:04:05"),
		time.Now().Format("2006/01/02 15:04:05"),
	)

	return
}

func (e PostEditor) Save(path string, file io.Reader) (err error) {
	defer e.db.Close()

	p, err := parsePostPath(path)
	if err != nil {
		return
	}

	if p.lang == "" || p.slug == "" {
		err = fmt.Errorf("invalid path")
		return
	}

	var data map[string]FormElement
	if err = json.NewDecoder(file).Decode(&data); err != nil {
		return
	}

	_, err = e.db.Exec(
		"UPDATE Posts SET published = true, title = ?, description = ?, content = ?",
		data["title"].Value,
		data["description"].Value,
		data["content"].Value,
	)

	return
}

func (e PostEditor) Meta(path string) Metadata {
	p, _ := parsePostPath(path)
	return Metadata{
		CanCreateDirectory: NewBool(false),
		CanCreateFile:      NewBool(p.slug == ""),
		CanRename:          NewBool(p.slug != ""),
		CanMove:            NewBool(false),
		RefreshOnCreate:    NewBool(true),
		HideExtension:      NewBool(true),
	}
}

func (e PostEditor) Close() error {
	return e.db.Close()
}

type postPath struct {
	lang string
	slug string
}

func parsePostPath(path string) (res postPath, err error) {
	p := strings.Split(strings.Trim(path, "/"), "/")

	if len(p) > 2 {
		err = fmt.Errorf("invalid path")
		return
	}

	res.lang = p[0]

	if len(p) > 1 {
		res.slug = p[1]
	}

	return
}
