package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"

	"github.com/disintegration/imaging"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/gorilla/mux"
	"github.com/skip2/go-qrcode"
	excel "github.com/xuri/excelize/v2"
	zipprotect "github.com/yeka/zip"
)

type filename string

func main() {
	// create the server
	router := mux.NewRouter().StrictSlash(true)
	fs := http.FileServer(http.Dir("src"))
	router.PathPrefix("/src").Handler(http.StripPrefix("/src", fs))
	router.HandleFunc("/", filename("./src/index.html").loadFile).Methods("GET")

	router.HandleFunc("/rotate/download", Rotate).Methods("POST")
	router.HandleFunc("/resize/download", Resize).Methods("POST")
	router.HandleFunc("/csv/excel/download", csvToExcel).Methods("POST")
	router.HandleFunc("/qrcode/download", QrGenerator).Methods("GET")
	router.HandleFunc("/barcode/download", BarCodeGenerator).Methods("GET")
	router.HandleFunc("/zip/download", zipConvert).Methods("POST")
	http.ListenAndServe(":8081", router)
}

func (fn filename) loadFile(w http.ResponseWriter, r *http.Request) {
	file, err := os.Open(string(fn))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.ServeContent(w, r, file.Name(), fileInfo.ModTime(), file)
}

func QrGenerator(w http.ResponseWriter, r *http.Request) {
	content := r.URL.Query().Get("content")
	fmt.Println("content", content)
	qrcodeByte, err := qrcode.Encode(content, qrcode.Medium, 256)
	if ErrorCheck(w, 500, err) {
		return
	}
	respondWithDownload(w, qrcodeByte)
}

func BarCodeGenerator(w http.ResponseWriter, r *http.Request) {
	content := r.URL.Query().Get("content")

	buf, err := barcodeProcess(content)
	if ErrorCheck(w, 500, err) {
		return
	}
	respondWithDownload(w, buf.Bytes())
}

func Resize(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("image")
	if ErrorCheck(w, 400, err) {
		return
	}
	query := r.URL.Query()
	width, err := strconv.Atoi(query.Get("width"))
	if ErrorCheck(w, 400, err) {
		return
	}
	height, err := strconv.Atoi(query.Get("height"))
	if ErrorCheck(w, 400, err) {
		return
	}
	decodeImage, err := imaging.Decode(file)
	if ErrorCheck(w, 500, err) {
		return
	}
	ResizedImage := imaging.Resize(decodeImage, width, height, imaging.Lanczos)
	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, ResizedImage, &jpeg.Options{Quality: 100})
	if ErrorCheck(w, 500, err) {
		return
	}
	respondWithDownload(w, buf.Bytes())
}

func Rotate(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("image")
	if ErrorCheck(w, 400, err) {
		return
	}
	query := r.URL.Query()
	deg, err := strconv.Atoi(query.Get("deg"))
	if ErrorCheck(w, 400, err) {
		return
	}
	decodeImage, err := imaging.Decode(file)
	if ErrorCheck(w, 500, err) {
		return
	}
	ResizedImage := imaging.Rotate(decodeImage, float64(deg), color.Transparent)
	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, ResizedImage, &jpeg.Options{Quality: 100})
	if ErrorCheck(w, 500, err) {
		return
	}
	respondWithDownload(w, buf.Bytes())
}

func barcodeProcess(content string) (*bytes.Buffer, error) {
	bcode, err := code128.Encode(content)
	if err != nil {
		return nil, err
	}

	barcode, err := barcode.Scale(bcode, bcode.Bounds().Max.X-bcode.Bounds().Min.X, 40)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, barcode, &jpeg.Options{Quality: 100})
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func csvToExcel(w http.ResponseWriter, r *http.Request) {
	csvFile, _, err := r.FormFile("csv")
	if ErrorCheck(w, 400, err) {
		fmt.Println("error :", err)
		return
	}
	password := r.FormValue("password")
	colorcode := r.FormValue("colorcode")
	rec, err := csv.NewReader(csvFile).ReadAll()
	if ErrorCheck(w, 500, err) {
		return
	}
	excelFile := excel.NewFile()
	headerStyle, err := excelFile.NewStyle(&excel.Style{
		Border: []excel.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
		Fill: excel.Fill{Type: "pattern", Color: []string{colorcode}, Pattern: 1},
	})
	if ErrorCheck(w, 500, err) {
		return
	}
	contentStyle, err := excelFile.NewStyle(&excel.Style{
		Border: []excel.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})
	if ErrorCheck(w, 500, err) {
		return
	}
	colCount := 0
	for i, row := range rec {
		for j, col := range row {
			excelFile.SetCellValue("Sheet1", fmt.Sprintf("%c%d", 'A'+j, i+1), col)
		}
		colCount++
	}
	fmt.Println("len rec", fmt.Sprintf("A%d", colCount))
	fmt.Println("len rec", fmt.Sprintf("%c%d", 'A'+len(rec), colCount))
	excelFile.SetCellStyle("Sheet1", "A1", fmt.Sprintf("A%d", colCount), headerStyle)
	excelFile.SetCellStyle("Sheet1", "A2", fmt.Sprintf("%c%d", 'A'+len(rec), colCount), contentStyle)
	// excelFile.SaveAs("csvtoexc.xlsx")
	excelFile.Write(w, excel.Options{Password: password})

	setHeader(w, "output.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
}

func zipConvert(w http.ResponseWriter, r *http.Request) {
	files, password, err := getPayloadFiles(r)
	if ErrorCheck(w, 400, err) {
		return
	}
	err = ZipProtectionCompression(w, files, password)
	if ErrorCheck(w, 400, err) {
		return
	}
	setHeader(w, "output.zip", "application/zip")
}

func getPayloadFiles(r *http.Request) ([]*multipart.FileHeader, string, error) {
	// Parse our multipart form
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		return nil, "", err
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		return nil, "", errors.New("No files")
	}
	password := r.FormValue("password")
	return files, password, nil
}

func ZipProtectionCompression(w http.ResponseWriter, fileHeader []*multipart.FileHeader, password string) error {
	zipWriter := zipprotect.NewWriter(w)
	for _, file := range fileHeader {
		writer, err := zipWriter.Encrypt(file.Filename, password, zipprotect.AES256Encryption)
		if err != nil {
			return err
		}
		openFile, err := file.Open()
		if err != nil {
			return err
		}
		if _, err := io.Copy(writer, openFile); err != nil {
			return err
		}
		openFile.Close()
	}
	zipWriter.Close()
	return nil
}

// Error handling
func ErrorCheck(w http.ResponseWriter, statusCode int, err error) bool {
	if err != nil {
		response, _ := json.Marshal(map[string]interface{}{
			"error": err.Error(),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(response)
		return true
	}
	return false
}

func respondWithJson(w http.ResponseWriter, m interface{}) {
	jsn, _ := json.Marshal(m)

	// Set the custom response header
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(jsn)
}

func setHeader(w http.ResponseWriter, filename, contentType string) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	// Set the custom res header
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(200)
}

func respondWithDownload(w http.ResponseWriter, byt []byte) {
	w.Header().Set("Content-Disposition", "attachment;filename=output.jpg")
	w.Header().Set("Content-Type", "image/jpeg")
	w.WriteHeader(200)
	w.Write(byt)
}
