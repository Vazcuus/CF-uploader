package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq" // Драйвер Postgres
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Структура для отображения файла из БД
type FileRecord struct {
	ID           int
	Filename     string
	S3Key        string
	UploaderName string
	FileSize     int64
	UploadDate   time.Time
}

var (
	// S3 переменные
	endpoint        = os.Getenv("S3_ENDPOINT")
	accessKeyID     = os.Getenv("S3_ACCESS_KEY")
	secretAccessKey = os.Getenv("S3_SECRET_KEY")
	useSSL          = os.Getenv("S3_USE_SSL") == "true"
	bucketName      = os.Getenv("S3_BUCKET")

	// Postgres переменные
	dbHost     = os.Getenv("DB_HOST")
	dbPort     = os.Getenv("DB_PORT")
	dbUser     = os.Getenv("DB_USER")
	dbPassword = os.Getenv("DB_PASSWORD")
	dbName     = os.Getenv("DB_NAME")
)

var tmpl = template.Must(template.New("index").Parse(`
<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>S3 Manager & Chat</title>
    <style>
        :root {
            --primary: #2563eb;
            --danger: #ef4444;
            --bg: #f8fafc;
            --text: #1e293b;
            --card-bg: #ffffff;
            --border: #e2e8f0;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background-color: var(--bg);
            color: var(--text);
            margin: 0;
            height: 100vh;
            display: flex;
            overflow: hidden;
        }

        /* Layout */
        .main-wrapper { display: flex; width: 100%; height: 100%; }
        
        /* Левая часть: Файлы */
        .files-column {
            flex: 1;
            padding: 30px 40px;
            overflow-y: auto;
            border-right: 1px solid var(--border);
        }

        /* Правая часть: Чат */
        .chat-column {
            width: 400px;
            background: var(--card-bg);
            display: flex;
            flex-direction: column;
            box-shadow: -2px 0 10px rgba(0,0,0,0.02);
        }

        /* Элементы управления файлами */
        .upload-section {
            background: #fff;
            padding: 1.5rem;
            border-radius: 12px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
            margin-bottom: 2rem;
            border: 1px solid var(--border);
        }
        .search-container { margin-bottom: 1.5rem; position: relative; }
        .search-input {
            width: 100%; padding: 12px 16px 12px 40px;
            border-radius: 10px; border: 1px solid var(--border);
            font-size: 0.95rem; outline: none; box-sizing: border-box;
        }
        .search-icon { position: absolute; left: 14px; top: 50%; transform: translateY(-50%); color: #94a3b8; }

        .btn { padding: 10px 20px; border-radius: 8px; font-weight: 500; cursor: pointer; border: none; font-size: 0.9rem; }
        .btn-primary { background: var(--primary); color: white; }
        .btn-danger { background: var(--danger); color: white; display: none; }

        /* Список файлов */
        .file-list { list-style: none; padding: 0; }
        .file-item {
            background: #fff; margin-bottom: 8px; padding: 12px 16px;
            border-radius: 10px; display: flex; justify-content: space-between;
            align-items: center; border: 1px solid var(--border);
        }
        .file-main { display: flex; align-items: center; gap: 15px; }
        .file-name { font-weight: 500; font-size: 0.95rem; }
        .file-size { font-size: 0.8rem; color: #64748b; }

        /* Чат */
        .chat-header { padding: 20px; border-bottom: 1px solid var(--border); }
        .chat-tabs { display: flex; gap: 5px; margin-top: 10px; }
        .tab-btn { padding: 6px 12px; font-size: 0.8rem; border-radius: 20px; border: 1px solid var(--border); cursor: pointer; }
        .tab-btn.active { background: var(--primary); color: white; border-color: var(--primary); }
        .chat-messages { flex: 1; padding: 20px; overflow-y: auto; display: flex; flex-direction: column; gap: 10px; }
        .msg { padding: 10px; border-radius: 10px; font-size: 0.9rem; background: #f1f5f9; max-width: 85%; }
        .chat-input-area { padding: 20px; border-top: 1px solid var(--border); display: flex; gap: 10px; }
        .chat-input { flex: 1; padding: 10px; border: 1px solid var(--border); border-radius: 8px; outline: none; }
    </style>
</head>
<body>
    <div class="main-wrapper">
        <div class="files-column">
            <h2>TellThink Storage</h2>
            <div class="upload-section">
                <form action="/upload" method="post" enctype="multipart/form-data" style="display: flex; gap: 10px;">
                    <input type="file" name="myFile" required>
                    <button type="submit" class="btn btn-primary">Загрузить</button>
                </form>
            </div>

            <form action="/delete" method="post" id="deleteForm">
                <h2 style="display: flex; justify-content: space-between;">
                    Файлы в облаке
                    <button type="submit" class="btn btn-danger" id="deleteBtn">Удалить</button>
                </h2>

                <div class="search-container">
                    <span class="search-icon">🔍</span>
                    <input type="text" id="searchInput" class="search-input" placeholder="Поиск по названию..." onkeyup="filterFiles()">
                </div>

                <ul class="file-list" id="fileList">
                {{range .}}
                    <li class="file-item">
                        <div class="file-main">
                            <input type="checkbox" name="names" value="{{.Filename}}" class="file-checkbox" onchange="updateUI()">
                            <div class="file-info">
                                <div class="file-name">{{.Filename}}</div>
                                <div class="file-size">{{.FileSize}} байт</div>
                            </div>
                        </div>
                        <a href="/download?name={{.Filename}}" style="text-decoration:none; color:var(--primary); font-weight:600; font-size:0.85rem;">Скачать</a>
                    </li>
                {{else}}
                    <p style="text-align:center; color:#94a3b8;">Файлов нет</p>
                {{end}}
                </ul>
            </form>
        </div>

        <div class="chat-column">
            <div class="chat-header">
                <h3 style="margin:0;">Обсуждение</h3>
                <div class="chat-tabs">
                    <button class="tab-btn active">Общее</button>
                    <button class="tab-btn">Важное</button>
                    <button class="tab-btn">Инфо</button>
                </div>
            </div>
            <div class="chat-messages" id="chatMsgs">
                <div class="msg">Чат готов к работе! 🚀</div>
            </div>
            <div class="chat-input-area">
                <input type="text" class="chat-input" placeholder="Сообщение...">
                <button class="btn btn-primary">➜</button>
            </div>
        </div>
    </div>

    <script>
        function filterFiles() {
            const filter = document.getElementById('searchInput').value.toLowerCase();
            const items = document.getElementsByClassName('file-item');
            for (let item of items) {
                const name = item.querySelector('.file-name').textContent.toLowerCase();
                item.style.display = name.includes(filter) ? "" : "none";
            }
        }

        function updateUI() {
            const count = document.querySelectorAll('.file-checkbox:checked').length;
            document.getElementById('deleteBtn').style.display = count > 0 ? 'block' : 'none';
        }
    </script>
</body>
</html>
`))

func main() {
	// 1. Инициализация MinIO
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatalln("MinIO Error:", err)
	}

	// 2. Инициализация Postgres
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalln("DB Connection Error:", err)
	}
	defer db.Close()

	// Главная страница: теперь берем данные из БД
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, filename, s3_key, filesize, upload_date FROM files ORDER BY upload_date DESC")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()

		var files []FileRecord
		for rows.Next() {
			var f FileRecord
			if err := rows.Scan(&f.ID, &f.Filename, &f.S3Key, &f.FileSize, &f.UploadDate); err != nil {
				continue
			}
			files = append(files, f)
		}
		tmpl.Execute(w, files)
	})

	// Загрузка: сначала в S3, потом запись в БД
	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", 303)
			return
		}
		file, handler, err := r.FormFile("myFile")
		if err != nil {
			http.Error(w, "Error file", 400)
			return
		}
		defer file.Close()

		// Загружаем в S3
		_, err = minioClient.PutObject(context.Background(), bucketName, handler.Filename, file, handler.Size, minio.PutObjectOptions{
			ContentType: handler.Header.Get("Content-Type"),
		})
		if err != nil {
			http.Error(w, "S3 Upload Error: "+err.Error(), 500)
			return
		}

		// Сохраняем метаданные в БД
		_, err = db.Exec("INSERT INTO files (filename, s3_key, filesize) VALUES ($1, $2, $3)",
			handler.Filename, handler.Filename, handler.Size)
		if err != nil {
			log.Println("DB Insert Error:", err)
		}

		http.Redirect(w, r, "/", 303)
	})

	// Скачивание (без изменений, использует S3_KEY из URL)
	http.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		objectName := r.URL.Query().Get("name")
		object, err := minioClient.GetObject(context.Background(), bucketName, objectName, minio.GetObjectOptions{})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer object.Close()
		w.Header().Set("Content-Disposition", "attachment; filename="+objectName)
		io.Copy(w, object)
	})

	// Удаление: и из S3, и из БД
	http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		fileNames := r.Form["names"]

		for _, name := range fileNames {
			// Удаляем из S3
			err := minioClient.RemoveObject(context.Background(), bucketName, name, minio.RemoveObjectOptions{})
			if err != nil {
				log.Println("S3 Delete Error:", err)
				continue
			}
			// Удаляем из БД
			_, err = db.Exec("DELETE FROM files WHERE s3_key = $1", name)
			if err != nil {
				log.Println("DB Delete Error:", err)
			}
		}
		http.Redirect(w, r, "/", 303)
	})

	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}