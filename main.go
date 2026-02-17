package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

// App holds shared dependencies.
type App struct {
	DB *sql.DB
}

var phonePattern = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

func main() {
	if err := loadDotEnv(); err != nil {
		log.Printf("warning: could not load .env: %v", err)
	}

	dsn := firstNonEmpty(
		os.Getenv("DATABASE_URL"),
		os.Getenv("POSTGRES_DSN"),
	)
	if dsn == "" {
		log.Fatal("DATABASE_URL or POSTGRES_DSN must be set")
	}

	db, err := openDB(dsn)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	app := &App{DB: db}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.rootHandler)
	mux.HandleFunc("/healthz", app.healthHandler)
	mux.HandleFunc("/contact", app.contactHandler)
	mux.HandleFunc("/banners", app.bannersHandler)
	mux.HandleFunc("/partners", app.partnersHandler)
	mux.HandleFunc("/tuning", app.tuningHandler)
	mux.HandleFunc("/service_offerings", app.serviceOfferingsHandler)
	mux.HandleFunc("/api/consultations", app.consultationsHandler)
	mux.HandleFunc("/portfolio_items", app.portfolioItemsHandler)
	mux.HandleFunc("/work_post", app.workPostHandler)

	server := &http.Server{
		Addr:         ":" + firstNonEmpty(os.Getenv("PORT"), "8080"),
		Handler:      loggingMiddleware(corsMiddleware(mux)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start the HTTP server.
	go func() {
		log.Printf("listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	if err := db.Close(); err != nil {
		log.Printf("database close error: %v", err)
	}
	log.Println("shutdown complete")
}

func (a *App) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := a.DB.PingContext(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","db":true}`))
}

func (a *App) bannersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rows, err := a.DB.QueryContext(
		ctx,
		`SELECT id, section, title, image_url, priority FROM banners ORDER BY priority ASC, id ASC`,
	)
	if err != nil {
		http.Error(w, "failed to fetch banners", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type banner struct {
		ID       int    `json:"id"`
		Section  string `json:"section"`
		Title    string `json:"title"`
		ImageURL string `json:"image_url"`
		Priority int    `json:"priority"`
	}

	banners := make([]banner, 0, 8)
	for rows.Next() {
		var b banner
		if err := rows.Scan(&b.ID, &b.Section, &b.Title, &b.ImageURL, &b.Priority); err != nil {
			http.Error(w, "failed to read banners", http.StatusInternalServerError)
			return
		}
		banners = append(banners, b)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read banners", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(banners); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) contactHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rows, err := a.DB.QueryContext(
		ctx,
		`SELECT id, phone_number, address, description, email, work_schedule
		FROM contact
		ORDER BY id ASC`,
	)
	if err != nil {
		rows, err = a.DB.QueryContext(
			ctx,
			`SELECT id, phone_number, address, description, NULL::text AS email, NULL::text AS work_schedule
			FROM contact_page
			ORDER BY id ASC`,
		)
	}
	if err != nil {
		http.Error(w, "failed to fetch contact", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type contact struct {
		ID           int     `json:"id"`
		PhoneNumber  *string `json:"phone_number"`
		Address      *string `json:"address"`
		Description  *string `json:"description"`
		Email        *string `json:"email"`
		WorkSchedule *string `json:"work_schedule"`
	}

	contacts := make([]contact, 0, 4)
	for rows.Next() {
		var c contact
		var phoneNumber sql.NullString
		var address sql.NullString
		var description sql.NullString
		var email sql.NullString
		var workSchedule sql.NullString

		if err := rows.Scan(
			&c.ID,
			&phoneNumber,
			&address,
			&description,
			&email,
			&workSchedule,
		); err != nil {
			http.Error(w, "failed to read contact", http.StatusInternalServerError)
			return
		}

		c.PhoneNumber = nullableString(phoneNumber)
		c.Address = nullableString(address)
		c.Description = nullableString(description)
		c.Email = nullableString(email)
		c.WorkSchedule = nullableString(workSchedule)

		contacts = append(contacts, c)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read contact", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(contacts); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) partnersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rows, err := a.DB.QueryContext(
		ctx,
		`SELECT id, logo_url FROM partners ORDER BY id ASC`,
	)
	if err != nil {
		http.Error(w, "failed to fetch partners", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type partner struct {
		ID      int    `json:"id"`
		LogoURL string `json:"logo_url"`
	}

	partners := make([]partner, 0, 8)
	for rows.Next() {
		var p partner
		if err := rows.Scan(&p.ID, &p.LogoURL); err != nil {
			http.Error(w, "failed to read partners", http.StatusInternalServerError)
			return
		}
		partners = append(partners, p)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read partners", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(partners); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) tuningHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	queries := []string{
		`SELECT id, to_jsonb(t)->>'brand' AS brand, to_jsonb(t)->>'model' AS model, NULL::text AS title, card_image_url, full_image_url, description, card_description, full_description, video_image_url, video_link, to_jsonb(t)->>'price' AS price, created_at, updated_at
		FROM public.tuning t
		ORDER BY created_at DESC, id DESC`,
		`SELECT id, to_jsonb(t)->>'brand' AS brand, to_jsonb(t)->>'model' AS model, title, card_image_url, full_image_url, title AS description, card_description, full_description, video_image_url, video_link, to_jsonb(t)->>'price' AS price, created_at, updated_at
		FROM public.tuning t
		ORDER BY created_at DESC, id DESC`,
		`SELECT id, to_jsonb(t)->>'brand' AS brand, to_jsonb(t)->>'model' AS model, NULL::text AS title, card_image_url, NULL::jsonb AS full_image_url, description, card_description, full_description, video_image_url, video_link, to_jsonb(t)->>'price' AS price, created_at, updated_at
		FROM public.tuning t
		ORDER BY created_at DESC, id DESC`,
		`SELECT id, to_jsonb(t)->>'brand' AS brand, to_jsonb(t)->>'model' AS model, title, card_image_url, NULL::jsonb AS full_image_url, title AS description, card_description, full_description, video_image_url, video_link, to_jsonb(t)->>'price' AS price, created_at, updated_at
		FROM public.tuning t
		ORDER BY created_at DESC, id DESC`,
		`SELECT row_number() OVER () AS id, to_jsonb(t)->>'brand' AS brand, to_jsonb(t)->>'model' AS model, NULL::text AS title, card_image_url, NULL::jsonb AS full_image_url, description, card_description, full_description, video_image_url, video_link, to_jsonb(t)->>'price' AS price, NOW() AS created_at, NOW() AS updated_at
		FROM public.tuning t`,
		`SELECT row_number() OVER () AS id, to_jsonb(t)->>'brand' AS brand, to_jsonb(t)->>'model' AS model, title, card_image_url, NULL::jsonb AS full_image_url, title AS description, card_description, full_description, video_image_url, video_link, to_jsonb(t)->>'price' AS price, NOW() AS created_at, NOW() AS updated_at
		FROM public.tuning t`,
		`SELECT id, to_jsonb(t)->>'brand' AS brand, to_jsonb(t)->>'model' AS model, NULL::text AS title, card_image_url, full_image_url, description, card_description, full_description, video_image_url, video_link, to_jsonb(t)->>'price' AS price, created_at, updated_at
		FROM public.tunning t
		ORDER BY created_at DESC, id DESC`,
		`SELECT id, to_jsonb(t)->>'brand' AS brand, to_jsonb(t)->>'model' AS model, title, card_image_url, full_image_url, title AS description, card_description, full_description, video_image_url, video_link, to_jsonb(t)->>'price' AS price, created_at, updated_at
		FROM public.tunning t
		ORDER BY created_at DESC, id DESC`,
	}

	var rows *sql.Rows
	var err error
	for _, query := range queries {
		rows, err = a.DB.QueryContext(ctx, query)
		if err == nil {
			break
		}
	}
	if err != nil {
		http.Error(w, "failed to fetch tuning", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type tuningItem struct {
		ID              int       `json:"id"`
		Brand           *string   `json:"brand"`
		Model           *string   `json:"model"`
		Title           *string   `json:"title"`
		CardImageURL    *string   `json:"card_image_url"`
		FullImageURL    []string  `json:"full_image_url"`
		Price           *string   `json:"price"`
		Description     *string   `json:"description"`
		CardDescription *string   `json:"card_description"`
		FullDescription *string   `json:"full_description"`
		VideoImageURL   *string   `json:"video_image_url"`
		VideoLink       *string   `json:"video_link"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}

	items := make([]tuningItem, 0, 8)
	for rows.Next() {
		var item tuningItem
		var brand sql.NullString
		var model sql.NullString
		var title sql.NullString
		var cardImageURL sql.NullString
		var fullImageURLRaw []byte
		var price sql.NullString
		var description sql.NullString
		var cardDescription sql.NullString
		var fullDescription sql.NullString
		var videoImageURL sql.NullString
		var videoLink sql.NullString

		if err := rows.Scan(
			&item.ID,
			&brand,
			&model,
			&title,
			&cardImageURL,
			&fullImageURLRaw,
			&description,
			&cardDescription,
			&fullDescription,
			&videoImageURL,
			&videoLink,
			&price,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			http.Error(w, "failed to read tuning", http.StatusInternalServerError)
			return
		}

		item.Brand = nullableString(brand)
		item.Model = nullableString(model)
		item.Title = nullableString(title)
		item.CardImageURL = nullableString(cardImageURL)
		item.FullImageURL = parseStringArray(fullImageURLRaw)
		item.Price = nullableString(price)
		item.Description = nullableString(description)
		item.CardDescription = nullableString(cardDescription)
		item.FullDescription = nullableString(fullDescription)
		item.VideoImageURL = nullableString(videoImageURL)
		item.VideoLink = nullableString(videoLink)

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read tuning", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) portfolioItemsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rows, err := a.DB.QueryContext(
		ctx,
		`SELECT id, brand, title, image_url, description, youtube_link, created_at
		FROM portfolio_items
		ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		http.Error(w, "failed to fetch portfolio items", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type portfolioItem struct {
		ID          int       `json:"id"`
		Brand       *string   `json:"brand"`
		Title       string    `json:"title"`
		ImageURL    string    `json:"image_url"`
		Description *string   `json:"description"`
		YoutubeLink *string   `json:"youtube_link"`
		CreatedAt   time.Time `json:"created_at"`
	}

	items := make([]portfolioItem, 0, 8)
	for rows.Next() {
		var item portfolioItem
		var brand sql.NullString
		var description sql.NullString
		var youtubeLink sql.NullString

		if err := rows.Scan(
			&item.ID,
			&brand,
			&item.Title,
			&item.ImageURL,
			&description,
			&youtubeLink,
			&item.CreatedAt,
		); err != nil {
			http.Error(w, "failed to read portfolio items", http.StatusInternalServerError)
			return
		}

		item.Brand = nullableString(brand)
		item.Description = nullableString(description)
		item.YoutubeLink = nullableString(youtubeLink)

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read portfolio items", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) workPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	tableName, err := resolveWorkPostTable(ctx, a.DB)
	if err != nil {
		http.Error(w, "failed to resolve work posts table", http.StatusInternalServerError)
		return
	}
	if tableName == "" {
		http.Error(w, "work posts table not found", http.StatusInternalServerError)
		return
	}

	hasGalleryImages, err := hasColumn(ctx, a.DB, tableName, "gallery_images")
	if err != nil {
		http.Error(w, "failed to resolve work posts columns", http.StatusInternalServerError)
		return
	}

	query := `SELECT id, title_model, card_image_url, full_image_url, card_description, work_list, full_description, video_image_url, video_link, NULL::jsonb AS gallery_images, created_at, updated_at
		FROM blog_posts
		ORDER BY created_at DESC, id DESC`
	if tableName == "work_post" && hasGalleryImages {
		query = `SELECT id, title_model, card_image_url, full_image_url, card_description, work_list, full_description, video_image_url, video_link, gallery_images, created_at, updated_at
		FROM work_post
		ORDER BY created_at DESC, id DESC`
	} else if tableName == "work_post" {
		query = `SELECT id, title_model, card_image_url, full_image_url, card_description, work_list, full_description, video_image_url, video_link, NULL::jsonb AS gallery_images, created_at, updated_at
		FROM work_post
		ORDER BY created_at DESC, id DESC`
	} else if tableName == "blog_posts" && hasGalleryImages {
		query = `SELECT id, title_model, card_image_url, full_image_url, card_description, work_list, full_description, video_image_url, video_link, gallery_images, created_at, updated_at
		FROM blog_posts
		ORDER BY created_at DESC, id DESC`
	}

	rows, err := a.DB.QueryContext(
		ctx,
		query,
	)
	if err != nil {
		http.Error(w, "failed to fetch work posts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type workPost struct {
		ID              int      `json:"id"`
		Title           string   `json:"title"`
		Description     string   `json:"description"`
		FullDescription string   `json:"fullDescription"`
		ImageURL        string   `json:"imageUrl"`
		VideoURL        string   `json:"videoUrl"`
		PerformedWorks  []string `json:"performedWorks"`
		GalleryImages   []string `json:"galleryImages"`
	}

	posts := make([]workPost, 0, 8)
	for rows.Next() {
		var post workPost
		var titleModel string
		var cardImageURL sql.NullString
		var fullImageURL sql.NullString
		var cardDescription sql.NullString
		var workList []byte
		var fullDescription sql.NullString
		var videoImageURL sql.NullString
		var videoLink sql.NullString
		var galleryImagesRaw []byte
		var createdAt time.Time
		var updatedAt time.Time

		if err := rows.Scan(
			&post.ID,
			&titleModel,
			&cardImageURL,
			&fullImageURL,
			&cardDescription,
			&workList,
			&fullDescription,
			&videoImageURL,
			&videoLink,
			&galleryImagesRaw,
			&createdAt,
			&updatedAt,
		); err != nil {
			http.Error(w, "failed to read work posts", http.StatusInternalServerError)
			return
		}

		cardURL := nullStringValue(cardImageURL)
		fullURL := nullStringValue(fullImageURL)
		videoImage := nullStringValue(videoImageURL)

		post.Title = titleModel
		post.Description = nullStringValue(cardDescription)
		post.FullDescription = nullStringValue(fullDescription)
		post.ImageURL = firstNonEmpty(cardURL, fullURL, videoImage)
		post.VideoURL = nullStringValue(videoLink)
		post.PerformedWorks = parsePerformedWorks(workList)
		post.GalleryImages = parseStringArray(galleryImagesRaw)
		if len(post.GalleryImages) == 0 {
			post.GalleryImages = uniqueNonEmpty(cardURL, fullURL, videoImage)
		}

		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read work posts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(posts); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) serviceOfferingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	queries := []string{
		`SELECT id, service_type, title, detailed_description, gallery_images, price_text, position, created_at, updated_at
		FROM public.service_offerings
		ORDER BY position ASC, id ASC`,
		`SELECT id, service_type, title, detailed_description, gallery_images, price_text, position, NOW() AS created_at, NOW() AS updated_at
		FROM public.service_offerings
		ORDER BY position ASC, id ASC`,
		`SELECT id, service_type, title, detailed_description, NULL::jsonb AS gallery_images, price_text, position, NOW() AS created_at, NOW() AS updated_at
		FROM public.service_offerings
		ORDER BY position ASC, id ASC`,
	}

	var rows *sql.Rows
	var err error
	for _, query := range queries {
		rows, err = a.DB.QueryContext(ctx, query)
		if err == nil {
			break
		}
	}
	if err != nil {
		http.Error(w, "failed to fetch service offerings", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type serviceOffering struct {
		ID                  int       `json:"id"`
		ServiceType         *string   `json:"service_type"`
		Title               *string   `json:"title"`
		DetailedDescription *string   `json:"detailed_description"`
		GalleryImages       []string  `json:"gallery_images"`
		PriceText           *string   `json:"price_text"`
		Position            int       `json:"position"`
		CreatedAt           time.Time `json:"created_at"`
		UpdatedAt           time.Time `json:"updated_at"`
	}

	items := make([]serviceOffering, 0, 8)
	for rows.Next() {
		var item serviceOffering
		var serviceType sql.NullString
		var title sql.NullString
		var detailedDescription sql.NullString
		var galleryImagesRaw []byte
		var priceText sql.NullString

		if err := rows.Scan(
			&item.ID,
			&serviceType,
			&title,
			&detailedDescription,
			&galleryImagesRaw,
			&priceText,
			&item.Position,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			http.Error(w, "failed to read service offerings", http.StatusInternalServerError)
			return
		}

		item.ServiceType = nullableString(serviceType)
		item.Title = nullableString(title)
		item.DetailedDescription = nullableString(detailedDescription)
		item.GalleryImages = parseStringArray(galleryImagesRaw)
		item.PriceText = nullableString(priceText)

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read service offerings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (a *App) consultationsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.createConsultationHandler(w, r)
	case http.MethodGet:
		a.listConsultationsHandler(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"status":  "error",
			"message": "Метод не поддерживается",
		})
	}
}

func (a *App) createConsultationHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB safety limit for JSON payload
	defer r.Body.Close()

	type consultationCreateRequest struct {
		FirstName         string `json:"first_name"`
		LastName          string `json:"last_name"`
		Phone             string `json:"phone"`
		ServiceType       string `json:"service_type"`
		CarModel          string `json:"car_model"`
		PreferredCallTime string `json:"preferred_call_time"`
		Comments          string `json:"comments"`
	}

	var req consultationCreateRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status":  "error",
			"message": "Некорректный JSON",
			"errors": map[string]string{
				"body": "Проверьте формат запроса",
			},
		})
		return
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status":  "error",
			"message": "Некорректный JSON",
			"errors": map[string]string{
				"body": "Ожидается один JSON-объект",
			},
		})
		return
	}

	errorsMap := validateConsultationRequest(
		req.FirstName,
		req.LastName,
		req.Phone,
		req.ServiceType,
		req.CarModel,
		req.PreferredCallTime,
		req.Comments,
	)
	if len(errorsMap) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"status":  "error",
			"message": "Ошибка валидации",
			"errors":  errorsMap,
		})
		return
	}

	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	phone := strings.TrimSpace(req.Phone)
	serviceType := strings.TrimSpace(req.ServiceType)
	carModel := optionalStringDBValue(req.CarModel)
	preferredCallTime := optionalStringDBValue(req.PreferredCallTime)
	comments := optionalStringDBValue(req.Comments)

	var id int64
	var createdAt time.Time
	err := a.DB.QueryRowContext(
		ctx,
		`INSERT INTO public.consultations
		(first_name, last_name, phone, service_type, car_model, preferred_call_time, comments, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'new')
		RETURNING id, created_at`,
		firstName,
		lastName,
		phone,
		serviceType,
		carModel,
		preferredCallTime,
		comments,
	).Scan(&id, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"message": "Не удалось сохранить заявку",
		})
		return
	}

	go notifyAdminAboutConsultation(consultationNotification{
		ID:                id,
		FirstName:         firstName,
		LastName:          lastName,
		Phone:             phone,
		ServiceType:       serviceType,
		CarModel:          optionalStringValue(req.CarModel),
		PreferredCallTime: optionalStringValue(req.PreferredCallTime),
		Comments:          optionalStringValue(req.Comments),
		Status:            "new",
		CreatedAt:         createdAt,
	})

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":  "success",
		"message": "Заявка успешно создана",
		"data": map[string]any{
			"id":         id,
			"created_at": createdAt.UTC().Format(time.RFC3339),
		},
	})
}

func (a *App) listConsultationsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	query := `SELECT id, first_name, last_name, phone, service_type, car_model, preferred_call_time, comments, status, created_at
		FROM public.consultations`
	args := []any{}
	if statusFilter != "" {
		query += ` WHERE status = $1`
		args = append(args, statusFilter)
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := a.DB.QueryContext(ctx, query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"message": "Не удалось получить заявки",
		})
		return
	}
	defer rows.Close()

	type consultationItem struct {
		ID                int64     `json:"id"`
		FirstName         string    `json:"first_name"`
		LastName          string    `json:"last_name"`
		Phone             string    `json:"phone"`
		ServiceType       string    `json:"service_type"`
		CarModel          *string   `json:"car_model"`
		PreferredCallTime *string   `json:"preferred_call_time"`
		Comments          *string   `json:"comments"`
		Status            string    `json:"status"`
		CreatedAt         time.Time `json:"created_at"`
	}

	items := make([]consultationItem, 0, 16)
	for rows.Next() {
		var item consultationItem
		var carModel sql.NullString
		var preferredCallTime sql.NullString
		var comments sql.NullString

		if err := rows.Scan(
			&item.ID,
			&item.FirstName,
			&item.LastName,
			&item.Phone,
			&item.ServiceType,
			&carModel,
			&preferredCallTime,
			&comments,
			&item.Status,
			&item.CreatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"status":  "error",
				"message": "Не удалось прочитать заявки",
			})
			return
		}

		item.CarModel = nullableString(carModel)
		item.PreferredCallTime = nullableString(preferredCallTime)
		item.Comments = nullableString(comments)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"message": "Не удалось прочитать заявки",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"data":   items,
	})
}

// rootHandler gives a friendly response for "/" instead of 404.
func (a *App) rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"service":"carbon_go","status":"running","routes":["/","/healthz","/contact","/banners","/partners","/tuning","/service_offerings","/api/consultations","/portfolio_items","/work_post"]}`))
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func loadDotEnv() error {
	// Load silently if file is absent so containers can rely on injected env.
	if _, err := os.Stat(".env"); err != nil {
		return nil
	}
	return godotenv.Load()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func nullableString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullStringValue(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return strings.TrimSpace(v.String)
}

func uniqueNonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func parsePerformedWorks(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}

	var asStrings []string
	if err := json.Unmarshal(raw, &asStrings); err == nil {
		return uniqueNonEmpty(asStrings...)
	}

	var asAny []any
	if err := json.Unmarshal(raw, &asAny); err != nil {
		return []string{}
	}

	works := make([]string, 0, len(asAny))
	for _, item := range asAny {
		switch v := item.(type) {
		case string:
			works = append(works, v)
		case map[string]any:
			for _, key := range []string{"step", "title", "name", "text", "description"} {
				val, ok := v[key]
				if !ok {
					continue
				}
				text, ok := val.(string)
				if ok && strings.TrimSpace(text) != "" {
					works = append(works, text)
					break
				}
			}
		}
	}

	return uniqueNonEmpty(works...)
}

func parseStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}

	var items []string
	if err := json.Unmarshal(raw, &items); err == nil {
		result := make([]string, 0, len(items))
		for _, item := range items {
			clean := strings.TrimSpace(item)
			if clean == "" {
				continue
			}
			result = append(result, clean)
		}
		return result
	}

	// Handle case when JSONB contains a string with encoded JSON array.
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		var nested []string
		if err := json.Unmarshal([]byte(encoded), &nested); err == nil {
			result := make([]string, 0, len(nested))
			for _, item := range nested {
				clean := strings.TrimSpace(item)
				if clean == "" {
					continue
				}
				result = append(result, clean)
			}
			return result
		}
	}

	return []string{}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func optionalStringDBValue(input string) any {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func optionalStringValue(input string) *string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validateConsultationRequest(firstName, lastName, phone, serviceType, carModel, preferredCallTime, comments string) map[string]string {
	errs := map[string]string{}

	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	phone = strings.TrimSpace(phone)
	serviceType = strings.TrimSpace(serviceType)
	carModel = strings.TrimSpace(carModel)
	preferredCallTime = strings.TrimSpace(preferredCallTime)
	comments = strings.TrimSpace(comments)

	if firstName == "" {
		errs["first_name"] = "Поле обязательно"
	} else if len([]rune(firstName)) > 100 {
		errs["first_name"] = "Максимум 100 символов"
	}

	if lastName == "" {
		errs["last_name"] = "Поле обязательно"
	} else if len([]rune(lastName)) > 100 {
		errs["last_name"] = "Максимум 100 символов"
	}

	if phone == "" {
		errs["phone"] = "Поле обязательно"
	} else if !phonePattern.MatchString(phone) {
		errs["phone"] = "Некорректный формат номера"
	}

	if serviceType == "" {
		errs["service_type"] = "Поле обязательно"
	} else if len([]rune(serviceType)) > 80 {
		errs["service_type"] = "Максимум 80 символов"
	}

	if len([]rune(carModel)) > 120 {
		errs["car_model"] = "Максимум 120 символов"
	}
	if len([]rune(preferredCallTime)) > 120 {
		errs["preferred_call_time"] = "Максимум 120 символов"
	}
	if len([]rune(comments)) > 2000 {
		errs["comments"] = "Максимум 2000 символов"
	}

	return errs
}

type consultationNotification struct {
	ID                int64     `json:"id"`
	FirstName         string    `json:"first_name"`
	LastName          string    `json:"last_name"`
	Phone             string    `json:"phone"`
	ServiceType       string    `json:"service_type"`
	CarModel          *string   `json:"car_model"`
	PreferredCallTime *string   `json:"preferred_call_time"`
	Comments          *string   `json:"comments"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

func notifyAdminAboutConsultation(payload consultationNotification) {
	webhookURL := strings.TrimSpace(os.Getenv("ADMIN_NOTIFY_WEBHOOK_URL"))
	if webhookURL == "" {
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("consultation notify marshal error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("consultation notify request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		log.Printf("consultation notify send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("consultation notify non-2xx status: %s", resp.Status)
	}
}

func resolveWorkPostTable(ctx context.Context, db *sql.DB) (string, error) {
	var workPost sql.NullString
	var blogPosts sql.NullString

	err := db.QueryRowContext(
		ctx,
		`SELECT to_regclass('public.work_post')::text, to_regclass('public.blog_posts')::text`,
	).Scan(&workPost, &blogPosts)
	if err != nil {
		return "", err
	}

	if workPost.Valid && strings.TrimSpace(workPost.String) != "" {
		return "work_post", nil
	}
	if blogPosts.Valid && strings.TrimSpace(blogPosts.String) != "" {
		return "blog_posts", nil
	}
	return "", nil
}

func resolveTuningTable(ctx context.Context, db *sql.DB) (string, error) {
	var tuning sql.NullString
	var tunning sql.NullString

	err := db.QueryRowContext(
		ctx,
		`SELECT to_regclass('public.tuning')::text, to_regclass('public.tunning')::text`,
	).Scan(&tuning, &tunning)
	if err != nil {
		return "", err
	}

	if tuning.Valid && strings.TrimSpace(tuning.String) != "" {
		return "tuning", nil
	}
	if tunning.Valid && strings.TrimSpace(tunning.String) != "" {
		return "tunning", nil
	}
	return "", nil
}

func hasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = $1
			  AND column_name = $2
		)`,
		tableName,
		columnName,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// loggingMiddleware adds a minimal request log.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Truncate(time.Millisecond))
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
