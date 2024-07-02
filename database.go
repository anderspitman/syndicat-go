package syndicat

import (
	//"errors"
	//"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type Database interface {
}

type SqliteDatabase struct {
	sdb *sqlx.DB
}

type Entry struct {
	Id            int
	Title         string
	Author        string
	PublishedTime time.Time
	ModifiedTime  time.Time
	Format        string
	Content       string
	Tags          []string
}

type DbConfig struct {
	JwksJson string `json:"jwks_json"`
}

func NewDatabase(dbPath string) (*SqliteDatabase, error) {

	sdb, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	stmt := `
        PRAGMA foreign_keys = ON;
        `
	_, err = sdb.Exec(stmt)
	if err != nil {
		return nil, err
	}

	stmt = `
        CREATE TABLE IF NOT EXISTS config(
                jwks_json TEXT
        );
        `
	_, err = sdb.Exec(stmt)
	if err != nil {
		return nil, err
	}

	stmt = `
        SELECT COUNT(*) FROM config;
        `
	var numRows int
	err = sdb.QueryRow(stmt).Scan(&numRows)
	if err != nil {
		return nil, err
	}

	if numRows == 0 {
		stmt = `
                INSERT INTO config (jwks_json) VALUES("");
                `
		_, err = sdb.Exec(stmt)
		if err != nil {
			return nil, err
		}
	}

	stmt = `
        CREATE TABLE IF NOT EXISTS entries(
                id INTEGER PRIMARY KEY,
                title TEXT,
                format TEXT,
                content TEXT,
                timestamp TEXT
        );
        `
	_, err = sdb.Exec(stmt)
	if err != nil {
		return nil, err
	}

	stmt = `
        CREATE TABLE IF NOT EXISTS tags(
                tag TEXT,
                entry_id TEXT,
                FOREIGN KEY(entry_id) REFERENCES entries(id)
        );
        `
	_, err = sdb.Exec(stmt)
	if err != nil {
		return nil, err
	}
	db := &SqliteDatabase{
		sdb: sdb,
	}

	return db, nil
}

func (d *SqliteDatabase) GetConfig() (*DbConfig, error) {
	var c DbConfig

	stmt := "SELECT jwks_json FROM config"
	err := d.sdb.QueryRow(stmt).Scan(&c.JwksJson)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (d *SqliteDatabase) SetJwksJson(jwksJson string) error {
	stmt := `
        UPDATE config SET jwks_json=?;
        `
	_, err := d.sdb.Exec(stmt, jwksJson)
	if err != nil {
		return err
	}

	return nil
}

func (d *SqliteDatabase) AddEntry(e *Entry) error {
	stmt := `
        INSERT INTO entries(id,title,format,content) VALUES(?,?,?,?);
        `
	_, err := d.sdb.Exec(stmt, e.Id, e.Title, e.Format, e.Content)
	if err != nil {
		return err
	}

	for _, tag := range e.Tags {
		stmt := `
                INSERT INTO tags(tag,entry_id) VALUES(?,?);
                `
		_, err := d.sdb.Exec(stmt, tag, e.Id)
		if err != nil {
			return err
		}
	}

	return nil
}
