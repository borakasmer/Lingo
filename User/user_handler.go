package User

import (
	"encoding/json"
	"log"
	"net/http"
	"sqlOrm/Repository"
	"strconv"
	"strings"

	"sqlOrm/Models"
)

type UserHandler struct {
	repository *UserRepository
}

func NewUserHandler(repository *UserRepository) *UserHandler {
	return &UserHandler{repository: repository}
}

// GetByName godoc
// @Summary İsme göre kullanıcı ara
// @Description Adında aranan ifade geçen aktif kullanıcıları listeler.
// @Tags Users
// @Produce json
// @Param name query string true "Aranacak isim" example(Bora)
// @Success 200 {array} Models.Person
// @Failure 400 {string} string "name parametresi zorunludur"
// @Failure 405 {string} string "Method not allowed"
// @Failure 500 {string} string "Kullanıcılar sorgulanamadı"
// @Router /api/users [get]
func (h *UserHandler) GetByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		http.Error(w, "name parametresi zorunludur", http.StatusBadRequest)
		return
	}

	users, err := h.repository.GetByName(r.Context(), name)
	if err != nil {
		log.Printf("Kullanıcı sorgusu başarısız: %v", err)
		http.Error(
			w,
			"Kullanıcılar sorgulanamadı",
			http.StatusInternalServerError,
		)
		return
	}

	var response []Models.Person = users

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("JSON yanıtı yazılamadı: %v", err)
	}
}

// SearchUsers godoc
// @Summary AND ve OR koşullarıyla kullanıcı ara
// @Description Cinsiyet eşleşmeli **VE** (yaş belirtilen sınırdan büyük olmalı **VEYA** isim aranan ifadeyi içermeli).
// @Tags Users
// @Produce json
// @Param gender query string true "Cinsiyet" Enums(Male, Female)
// @Param age query int true "Yaş alt sınırı; bu değerden büyük" minimum(0) example(30)
// @Param name query string true "İsimde aranacak ifade" example(Bor)
// @Success 200 {array} Models.Person
// @Failure 400 {string} string "Geçersiz arama parametreleri"
// @Failure 405 {string} string "Method not allowed"
// @Failure 500 {string} string "Kullanıcılar sorgulanamadı"
// @Router /api/users/search [get]
func (h *UserHandler) SearchUsers(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	gender := strings.TrimSpace(r.URL.Query().Get("gender"))
	if gender != "Male" && gender != "Female" {
		http.Error(w, "gender Male veya Female olmalı", http.StatusBadRequest)
		return
	}

	age, err := strconv.Atoi(r.URL.Query().Get("age"))
	if err != nil || age < 0 {
		http.Error(w, "age sıfır veya pozitif bir tam sayı olmalı", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		http.Error(w, "name parametresi zorunludur", http.StatusBadRequest)
		return
	}

	users, err := h.repository.SearchUsers(
		r.Context(),
		gender,
		age,
		name,
	)
	if err != nil {
		log.Printf("Kullanıcı araması başarısız: %v", err)
		http.Error(w, "Kullanıcılar sorgulanamadı", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(users); err != nil {
		log.Printf("JSON yanıtı yazılamadı: %v", err)
	}
}

// GetAllWithPaging godoc
// @Summary Kullanıcıları sayfalayarak ve sıralayarak getir
// @Description Aktif kullanıcıları listeler. sortBy boşsa Id kullanılır.
// @Tags Users
// @Produce json
// @Param page query int false "Sayfa numarası" default(1) minimum(1)
// @Param pageSize query int false "Sayfa başına kayıt" default(10) minimum(1) maximum(100)
// @Param sortBy query string false "Sıralama alanı: Id, Name, Age, Gender" default(Id)
// @Param ascending query bool false "true: artan, false: azalan" default(true)
// @Success 200 {array} Models.Person
// @Failure 400 {string} string "Geçersiz parametre"
// @Failure 405 {string} string "Method not allowed"
// @Failure 500 {string} string "Kullanıcılar sorgulanamadı"
// @Router /api/users/paging [get]
func (h *UserHandler) GetAllWithPaging(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()

	page := 1
	if value := query.Get("page"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			http.Error(w, "page tam sayı olmalı", http.StatusBadRequest)
			return
		}
		page = parsed
	}

	pageSize := 10
	if value := query.Get("pageSize"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			http.Error(w, "pageSize tam sayı olmalı", http.StatusBadRequest)
			return
		}
		pageSize = parsed
	}

	sortBy := strings.TrimSpace(query.Get("sortBy"))
	if sortBy == "" {
		sortBy = "Id"
	}

	// Bu endpoint'te sunacağımız sıralama alanları.
	switch strings.ToLower(sortBy) {
	case "id":
		sortBy = "Id"
	case "name":
		sortBy = "Name"
	case "age":
		sortBy = "Age"
	case "gender":
		sortBy = "Gender"
	default:
		http.Error(
			w,
			"sortBy: Id, Name, Age veya Gender olmalı",
			http.StatusBadRequest,
		)
		return
	}

	ascending := true
	if value := query.Get("ascending"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			http.Error(
				w,
				"ascending true veya false olmalı",
				http.StatusBadRequest,
			)
			return
		}
		ascending = parsed
	}

	users, err := h.repository.GetAllWithPaging(
		r.Context(),
		page,
		pageSize,
		sortBy,
		ascending,
	)
	if err != nil {
		log.Printf("Sayfalama sorgusu başarısız: %v", err)
		http.Error(
			w,
			"Kullanıcılar sorgulanamadı",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(users); err != nil {
		log.Printf("JSON yanıtı yazılamadı: %v", err)
	}
}

type BatchUpdateResponse struct {
	UpdatedCount int64 `json:"updatedCount"`
}

// AppendXToNamesOver40 godoc
// @Summary Yaşı 40'tan büyük kullanıcıların isimlerine X ekle
// @Description Aktif kullanıcıların yalnızca değişen Name alanını günceller. Bütün gruplar tek transaction içinde çalışır.
// @Tags Users
// @Produce json
// @Param batchSize query int true "Batch boyutu" default(1) minimum(1)
// @Success 200 {object} BatchUpdateResponse
// @Failure 400 {string} string "Geçersiz batchSize"
// @Failure 405 {string} string "Method not allowed"
// @Failure 500 {string} string "Güncelleme tamamlanamadı"
// @Router /api/users/append-x-over-40 [post]
func (h *UserHandler) AppendXToNamesOver40(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	batchSize, err := strconv.Atoi(r.URL.Query().Get("batchSize"))
	if err != nil || batchSize < 1 {
		http.Error(
			w,
			"batchSize pozitif bir tam sayı olmalı",
			http.StatusBadRequest,
		)
		return
	}

	affected, err := h.repository.AppendXToNamesOver40(
		r.Context(),
		batchSize,
	)
	if err != nil {
		log.Printf("Batch güncelleme başarısız: %v", err)
		http.Error(
			w,
			"Güncelleme tamamlanamadı",
			http.StatusInternalServerError,
		)
		return
	}

	response := BatchUpdateResponse{
		UpdatedCount: affected,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("JSON yanıtı yazılamadı: %v", err)
	}
}

// GetUserPermissions godoc
// @Summary Kullanıcının controller yetkilerini listele
// @Description INNER JOIN ile yetki bit maskesini okur ve örnek action tanımlarına göre isimlere çevirir.
// @Tags Users
// @Produce json
// @Param userID query int true "Kullanıcı ID" minimum(1) example(16)
// @Param controllerID query int true "Controller ID" minimum(1) example(1)
// @Success 200 {object} Models.UserPermissionResponse
// @Failure 400 {string} string "Parametreler pozitif tam sayı olmalı"
// @Failure 405 {string} string "Method not allowed"
// @Failure 500 {string} string "Yetkiler sorgulanamadı"
// @Router /api/users/permissions [get]
func (h *UserHandler) GetUserPermissions(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	values := r.URL.Query()

	userID, userErr := strconv.Atoi(values.Get("userID"))
	controllerID, controllerErr := strconv.Atoi(values.Get("controllerID"))

	if userErr != nil || controllerErr != nil ||
		userID <= 0 || controllerID <= 0 {
		http.Error(
			w,
			"Parametreler pozitif tam sayı olmalı",
			http.StatusBadRequest,
		)
		return
	}

	actions, err := h.repository.GetUserPermissions(
		r.Context(),
		userID,
		controllerID,
	)
	if err != nil {
		log.Printf("Kullanıcı yetkileri sorgulanamadı: %v", err)
		http.Error(
			w,
			"Yetkiler sorgulanamadı",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(
		Models.UserPermissionResponse{Actions: actions},
	); err != nil {
		log.Printf("JSON yanıtı yazılamadı: %v", err)
	}
}

type BulkInsertResponse struct {
	InsertedCount int64 `json:"insertedCount"`
}

// InsertSampleUsers godoc
// @Summary X, Y ve Z kullanıcılarını toplu ekle
// @Description Üç örnek kullanıcıyı BulkInsertBatched ile ekler. Her çağrıda yeni kayıtlar oluşturur.
// @Tags Users
// @Produce json
// @Success 200 {object} BulkInsertResponse
// @Failure 405 {string} string "Method not allowed"
// @Failure 500 {string} string "Kullanıcılar eklenemedi"
// @Router /api/users/bulk-insert [post]
func (h *UserHandler) InsertSampleUsers(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	count, err := h.repository.InsertSampleUsers(r.Context())
	if err != nil {
		log.Printf("Toplu kullanıcı ekleme başarısız: %v", err)
		http.Error(
			w,
			"Kullanıcılar eklenemedi",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(BulkInsertResponse{
		InsertedCount: count,
	}); err != nil {
		log.Printf("JSON yanıtı yazılamadı: %v", err)
	}
}

type DeleteUsersResponse struct {
	DeletedCount int64 `json:"deletedCount"`
}

// DeleteUsersFromID godoc
// @Summary ID sınırına göre kullanıcıları kalıcı sil
// @Description Id >= minID olan tüm kullanıcıları kalıcı siler. IsDeleted durumuna bakmaz.
// @Tags Users
// @Produce json
// @Param minID query int true "Silinecek minimum kullanıcı ID" minimum(1) example(40)
// @Success 200 {object} DeleteUsersResponse
// @Failure 400 {string} string "minID pozitif bir sayı olmalı"
// @Failure 405 {string} string "Method not allowed"
// @Failure 500 {string} string "Kullanıcılar silinemedi"
// @Router /api/users/bulk-delete [delete]
func (h *UserHandler) DeleteUsersFromID(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	minID, err := strconv.Atoi(r.URL.Query().Get("minID"))
	if err != nil || minID <= 0 {
		http.Error(
			w,
			"minID pozitif bir sayı olmalı",
			http.StatusBadRequest,
		)
		return
	}

	count, err := h.repository.DeleteUsersWithFilter(
		r.Context(),
		Repository.Filter{
			Field: "Id",
			Op:    Repository.OpGte,
			Value: minID,
		},
	)
	if err != nil {
		log.Printf("bulk delete users failed: %v", err)
		http.Error(
			w,
			"Users could not be deleted",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(DeleteUsersResponse{
		DeletedCount: count,
	}); err != nil {
		log.Printf("JSON yanıtı yazılamadı: %v", err)
	}
}
