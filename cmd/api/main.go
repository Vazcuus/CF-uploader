package main

import (
	"context"
	"fmt"
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
<html>
<head><title>S3 Manager</title></head>
<body>
    <h2>Загрузка файла</h2>
    <form action="/upload" method="post" enctype="multipart/form-data">
        <input type="file" name="myFile">
        <input type="submit" value="Загрузить">
    </form>
    <hr>
    <h2>Список файлов в S3</h2>
    <ul>
    {{range .}}
        <li>
            {{.Key}} ({{.Size}} байт) 
            <a href="/download?name={{.Key}}">Скачать</a>
        </li>
    {{else}}
        <li>Файлов пока нет</li>
    {{end}}
    </ul>
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

	log.Fatal(http.ListenAndServe(":8080", nil))
}