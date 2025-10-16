package main

import (
	"database/sql"
	"encoding/json"
	//"errors"
	//"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/go-sql-driver/mysql"
)

// Movie model for API + DB mapping
type Movie struct {
	ID          string    `json:"id" db:"id"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description" db:"description"`
	PosterURL   string    `json:"posterUrl,omitempty" db:"poster_url"`
	Genres      []Genre   `json:"genres,omitempty"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
}

type Genre struct {
	ID   string `json:"id" db:"id"`
	Name string `json:"name" db:"name"`
}

// Input for creating movie
type CreateMovieInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	PosterURL   string   `json:"posterUrl,omitempty"`
	GenreIDs    []string `json:"genreIds,omitempty"`
}

type App struct {
	db *sqlx.DB
}

func main() {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		log.Fatal("set MYSQL_DSN env, ex: user:111111@tcp(localhost:3306)/films?parseTime=true")
	}

	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("db ping error: %v", err)
	}

	app := &App{db: db}

	// ✅ создаем роутер
	r := chi.NewRouter()

	// ✅ подключаем статику и маршруты
	setupStaticFiles(r)
	//r.Get("/hall", hallDiagramHandler)
	r.Get("/hall", HallDiagramHandler)

	// ✅ уже существующие пути
	r.Post("/movies", app.createMovieHandler)
	r.Get("/movies", app.listMoviesHandler)
	r.Delete("/movies/{id}", app.deleteMovieHandler)

	// ✅ запуск сервера
	addr := ":8080"
	log.Printf("listening %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}


// Handler: create movie
func (a *App) createMovieHandler(w http.ResponseWriter, r *http.Request) {
	var in CreateMovieInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if in.Title == "" || in.Description == "" {
		http.Error(w, "title and description are required", http.StatusBadRequest)
		return
	}

	// validate genre IDs format (optional)
	for _, gid := range in.GenreIDs {
		if _, err := uuid.Parse(gid); err != nil {
			http.Error(w, "one or more genreIds are not valid UUIDs", http.StatusBadRequest)
			return
		}
	}

	tx, err := a.db.Beginx()
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	id := uuid.New().String()
	now := time.Now()

	// Insert movie
	_, err = tx.Exec(`INSERT INTO movies (id, title, description, poster_url, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, in.Title, in.Description, nullString(in.PosterURL), now)
	if err != nil {
		log.Printf("insert movie err: %v", err)
		http.Error(w, "failed to insert movie", http.StatusInternalServerError)
		return
	}

	// If genre IDs provided, insert pairs to movie_genres (validate FK)
	if len(in.GenreIDs) > 0 {
		stmt, err := tx.Preparex(`INSERT INTO movie_genres (movie_id, genre_id) VALUES (?, ?)`)
		if err != nil {
			log.Printf("prepare movie_genres err: %v", err)
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer stmt.Close()

		for _, gid := range in.GenreIDs {
			if gid == "" {
				continue
			}
			_, err := stmt.Exec(id, gid)
			if err != nil {
				// If FK fails, return 400 to client
				if isForeignKeyError(err) {
					http.Error(w, "one or more genreIds do not exist", http.StatusBadRequest)
					return
				}
				log.Printf("insert movie_genres err: %v", err)
				http.Error(w, "failed to associate genres", http.StatusInternalServerError)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("tx commit err: %v", err)
		http.Error(w, "failed to commit", http.StatusInternalServerError)
		return
	}

	created := Movie{
		ID:          id,
		Title:       in.Title,
		Description: in.Description,
		PosterURL:   in.PosterURL,
		CreatedAt:   now,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

// Handler: list movies (with genres)
func (a *App) listMoviesHandler(w http.ResponseWriter, r *http.Request) {
	// We'll build a map of movies and attach genres
	rows, err := a.db.Queryx(`
SELECT m.id,m.title,m.description,m.poster_url,m.created_at,
       g.id AS genre_id, g.name AS genre_name
FROM movies m
LEFT JOIN movie_genres mg ON mg.movie_id = m.id
LEFT JOIN genres g ON g.id = mg.genre_id
ORDER BY m.created_at DESC
`)
	if err != nil {
		log.Printf("query movies err: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	moviesMap := map[string]*Movie{}
	for rows.Next() {
		var (
			id, title, desc, poster sql.NullString
			created                sql.NullTime
			genreID, genreName     sql.NullString
		)
		if err := rows.Scan(&id, &title, &desc, &poster, &created, &genreID, &genreName); err != nil {
			log.Printf("scan err: %v", err)
			continue
		}
		if !id.Valid {
			continue
		}
		m := moviesMap[id.String]
		if m == nil {
			m = &Movie{
				ID:          id.String,
				Title:       title.String,
				Description: desc.String,
				PosterURL:   poster.String,
				Genres:      []Genre{},
				CreatedAt:   created.Time,
			}
			moviesMap[id.String] = m
		}
		if genreID.Valid {
			m.Genres = append(m.Genres, Genre{ID: genreID.String, Name: genreName.String})
		}
	}

	list := make([]Movie, 0, len(moviesMap))
	for _, mv := range moviesMap {
		list = append(list, *mv)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// Helpers

func isForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	// MySQL FK error messages include words like 'a foreign key constraint fails'
	// driver-specific checks are better, but simple string check is OK for demo
	se := strings.ToLower(err.Error())
	return strings.Contains(se, "foreign key") || strings.Contains(se, "a foreign key constraint")
}

func nullString(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}


// Handler: delete movie by id
func (a *App) deleteMovieHandler(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    id = strings.TrimSpace(id)
    if id == "" {
        http.Error(w, "missing movie id", http.StatusBadRequest)
        return
    }

    // perform delete
    res, err := a.db.Exec(`DELETE FROM movies WHERE id = ?`, id)
    if err != nil {
        // If there's a foreign key error (movie used in showtimes/reservations), return 409 Conflict
        if isForeignKeyError(err) {
            http.Error(w, "cannot delete movie: there are dependent records (showtimes/reservations). Please remove them first.", http.StatusConflict)
            return
        }
        log.Printf("delete movie err: %v", err)
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }

    // check rows affected
    rows, err := res.RowsAffected()
    if err != nil {
        log.Printf("rows affected err: %v", err)
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }
    if rows == 0 {
        http.Error(w, "movie not found", http.StatusNotFound)
        return
    }

    // success - no content
    w.WriteHeader(http.StatusNoContent)
}


func setupStaticFiles(r *chi.Mux) {
	staticDir := "./static"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		_ = os.Mkdir(staticDir, os.ModePerm)
	}
	fs := http.FileServer(http.Dir(staticDir))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))
}
