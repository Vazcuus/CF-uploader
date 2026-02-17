package main

import (
	"context"
	// "fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	endpoint        = os.Getenv("S3_ENDPOINT")
	accessKeyID     = os.Getenv("S3_ACCESS_KEY")
	secretAccessKey = os.Getenv("S3_SECRET_KEY")
	useSSL          = os.Getenv("S3_USE_SSL") == "true"
	bucketName      = os.Getenv("S3_BUCKET")
)

// Обновили шаблон: добавили список файлов с сылками на скачивание
var tmpl = template.Must(template.New("index").Parse(`
<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>S3 Storage</title>
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
            line-height: 1.6;
            margin: 0;
            display: flex;
            justify-content: center;
            padding: 40px 20px;
        }
        .container { width: 100%; max-width: 700px; }
        
        h2 { font-weight: 600; font-size: 1.5rem; margin-bottom: 1.2rem; display: flex; align-items: center; justify-content: space-between; }
        
        /* Секция загрузки */
        .upload-section {
            background: var(--card-bg);
            padding: 1.5rem;
            border-radius: 12px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
            margin-bottom: 2rem;
            border: 1px solid var(--border);
        }
        .upload-form { display: flex; gap: 10px; align-items: center; }
        input[type="file"] { font-size: 0.9rem; flex-grow: 1; }
        
        .btn {
            padding: 10px 20px;
            border-radius: 8px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s;
            border: none;
            font-size: 0.9rem;
        }
        .btn-primary { background: var(--primary); color: white; }
        .btn-primary:hover { background: #1d4ed8; }
        
        .btn-danger { 
            background: var(--danger); 
            color: white; 
            display: none; /* Скрыта пока не выбраны файлы */
        }
        .btn-danger:hover { background: #dc2626; }

        /* Список файлов */
        .file-list { list-style: none; padding: 0; margin: 0; }
        .file-item {
            background: var(--card-bg);
            margin-bottom: 8px;
            padding: 12px 16px;
            border-radius: 10px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            border: 1px solid var(--border);
            transition: transform 0.1s, border-color 0.2s;
        }
        .file-item:hover { border-color: #cbd5e1; background: #fdfdfd; }
        
        .file-main { display: flex; align-items: center; gap: 15px; }
        .file-checkbox { width: 18px; height: 18px; cursor: pointer; }
        
        .file-info { display: flex; flex-direction: column; }
        .file-name { font-weight: 500; font-size: 0.95rem; color: var(--text); }
        .file-size { font-size: 0.8rem; color: #64748b; }
        
        .download-link {
            text-decoration: none;
            color: var(--primary);
            font-size: 0.85rem;
            font-weight: 600;
            padding: 6px 12px;
            border-radius: 6px;
            background: #eff6ff;
        }
        .download-link:hover { background: #dbeafe; }

        .empty-state { text-align: center; padding: 40px; color: #94a3b8; background: #fff; border-radius: 12px; border: 1px dashed var(--border); }
    </style>
</head>
<body>
    <div class="container">
        <h2>S3 Storage</h2>
        
        <div class="upload-section">
            <form action="/upload" method="post" enctype="multipart/form-data" class="upload-form">
                <input type="file" name="myFile" required>
                <button type="submit" class="btn btn-primary">Загрузить</button>
            </form>
        </div>

        <form action="/delete" method="post" id="deleteForm">
            <h2>
                Файлы в облаке
                <button type="submit" class="btn btn-danger" id="deleteBtn">Удалить выбранные</button>
            </h2>
            
            <ul class="file-list">
            {{range .}}
                <li class="file-item">
                    <div class="file-main">
                        <input type="checkbox" name="names" value="{{.Key}}" class="file-checkbox" onchange="updateUI()">
                        <div class="file-info">
                            <span class="file-name">{{.Key}}</span>
                            <span class="file-size">{{.Size}} байт</span>
                        </div>
                    </div>
                    <a href="/download?name={{.Key}}" class="download-link">Скачать</a>
                </li>
            {{else}}
                <div class="empty-state">В этом бакете пока пусто. Загрузите первый файл!</div>
            {{end}}
            </ul>
        </form>
    </div>

    <script>
        function updateUI() {
            const checkedCount = document.querySelectorAll('.file-checkbox:checked').length;
            const deleteBtn = document.getElementById('deleteBtn');
            deleteBtn.style.display = checkedCount > 0 ? 'block' : 'none';
        }
    </script>
</body>
</html>
`))

func main() {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatalln(err)
	}

	// 1. Главная страница со списком файлов
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		var objects []minio.ObjectInfo
		
		// Получаем список объектов из бакета
		objectCh := minioClient.ListObjects(ctx, bucketName, minio.ListObjectsOptions{})
		for obj := range objectCh {
			if obj.Err != nil {
				http.Error(w, obj.Err.Error(), http.StatusInternalServerError)
				return
			}
			objects = append(objects, obj)
		}
		tmpl.Execute(w, objects)
	})

	// 2. Обработчик загрузки (как в прошлом примере)
	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		file, handler, err := r.FormFile("myFile")
		if err != nil {
			http.Error(w, "Error file", 400)
			return
		}
		defer file.Close()

		_, err = minioClient.PutObject(context.Background(), bucketName, handler.Filename, file, handler.Size, minio.PutObjectOptions{
			ContentType: handler.Header.Get("Content-Type"),
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// 3. Обработчик скачивания
	http.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		objectName := r.URL.Query().Get("name")
		if objectName == "" {
			http.Error(w, "Имя файла не указано", http.StatusBadRequest)
			return
		}

		// Получаем объект из S3
		object, err := minioClient.GetObject(context.Background(), bucketName, objectName, minio.GetObjectOptions{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer object.Close()

		// Устанавливаем заголовки, чтобы браузер начал скачивание файла
		w.Header().Set("Content-Disposition", "attachment; filename="+objectName)
		w.Header().Set("Content-Type", "application/octet-stream")

		// Копируем содержимое объекта в ответ HTTP
		if _, err := io.Copy(w, object); err != nil {
			log.Println("Ошибка при передаче файла:", err)
		}
	})
	
	http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Redirect(w, r, "/", http.StatusSeeOther)
        return
    }

    // Получаем список имен файлов из чекбоксов
    r.ParseForm()
    fileNames := r.Form["names"]

    if len(fileNames) > 0 {
        objectsCh := make(chan minio.ObjectInfo)

        // Отправляем имена в канал для удаления
        go func() {
            defer close(objectsCh)
            for _, name := range fileNames {
                objectsCh <- minio.ObjectInfo{Key: name}
            }
        }()

        // Выполняем удаление в MinIO
        errorCh := minioClient.RemoveObjects(context.Background(), bucketName, objectsCh, minio.RemoveObjectsOptions{})
        for err := range errorCh {
            log.Println("Ошибка при удалении объекта:", err.Err)
        }
    }

    http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}