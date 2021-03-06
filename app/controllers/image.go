package controllers

import (
	"github.com/valyala/fasthttp"
	"io"
	"strings"
	"strconv"
	"os"
	"gis/app"
	"gis/app/utils"
)

// 获取文件大小的接口
type Size interface {
	Size() int64
}
// 获取文件信息的接口
type Stat interface {
	Stat() (os.FileInfo, error)
}

type ImageController struct {
	BaseController
}

func NewImageController() *ImageController {
	return &ImageController{}
}

// 上传
func (this *ImageController) Upload(ctx *fasthttp.RequestCtx) {

	err := this.authToken(ctx)
	if err != nil {
		this.jsonError(ctx, err.Error(), nil)
		return
	}

	formField := app.Conf.GetString("upload.form_field")
	allowTypeSlice := app.Conf.GetStringSlice("upload.allow_type")
	rootDir := app.Conf.GetString("upload.root_dir")
	filenameLen := app.Conf.GetInt("upload.filename_len")
	dirNameLen := app.Conf.GetInt("upload.dirname_len")
	maxSize := app.Conf.GetInt("upload.max_size")
	thumbnails := app.Conf.GetStringSlice("upload.thumbnails")
	downloadUri := app.Conf.GetString("download.uri")
	imageUrl := downloadUri+"/image/"

	//req.ParseMultipartForm(4*1024)
	ctx.Request.MultipartForm()
	fileHeader, err := ctx.FormFile(formField)
	if err != nil {
		app.Log.Errorf("upload failed error: %S", err.Error())
		this.jsonError(ctx, "upload failed!", nil)
		return
	}
	fileObject, err := fileHeader.Open()
	if err != nil {
		app.Log.Errorf("upload failed error: %s", err.Error())
		this.jsonError(ctx, "upload failed!", nil)
		return
	}
	defer fileObject.Close()

	//文件上传格式判断
	filename := fileHeader.Filename
	ext := filename[strings.LastIndex(filename, "."):]
	isAllow := false
	for _, allowType := range allowTypeSlice {
		if strings.ToLower(allowType) == strings.ToLower(ext) {
			isAllow = true
			break
		}
	}
	if isAllow == false {
		app.Log.Errorf("upload image format %s not allow!", ext)
		this.jsonError(ctx, "upload image format not allow!", nil)
		return
	}

	// 判断上传文件大小
	if statInterface, ok := fileObject.(Stat); ok {
		fileInfo, _ := statInterface.Stat()
		size := fileInfo.Size()/1024
		if size > int64(maxSize) {
			app.Log.Errorf("upload image beyond maximum limit: %d kb", maxSize)
			this.jsonError(ctx, "upload image size "+ strconv.Itoa(int(size)) +" maximum limit! ", nil)
			return
		}
	}

	// 生成文件保存路径(防止发生随机碰撞)
	var i int
	var randString string
	var uploadPath string
	var saveFilename string
	for i = 0; i < 10; i++ {
		randString = strings.ToUpper(utils.GetRandomString(filenameLen))
		uploadPath = rootDir + utils.StringToPath(randString, dirNameLen)
		err = os.MkdirAll(uploadPath, 0755)
		if err != nil {
			app.Log.Errorf("create upload dir failed: %s", err.Error())
			this.jsonError(ctx, "create upload dir failed!", nil)
			return
		}
		saveFilename = uploadPath + "/" + randString + ext
		_, err = os.Stat(saveFilename)
		if os.IsNotExist(err) {
			break
		}
	}
	if i == 10 {
		app.Log.Errorf("create upload dir failed, random collision 10!")
		this.jsonError(ctx, "create upload dir failed!", nil)
		return
	}

	//将文件写入到指定的位置
	f, err := os.OpenFile(saveFilename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		app.Log.Errorf("save image file error: %s",  err.Error())
		this.jsonError(ctx, "save image file error!", nil)
		return
	}
	defer f.Close()
	io.Copy(f, fileObject)

	data := map[string]string{
		"image": imageUrl + randString + ext,
	}

	//开始生成缩略图
	for _, thumbnail := range thumbnails {
		thumbnailSlice := strings.Split(thumbnail, "_")
		if len(thumbnailSlice)  != 2 {
			continue
		}
		width, _:= strconv.Atoi(thumbnailSlice[0])
		height, _:= strconv.Atoi(thumbnailSlice[1])
		thumbSaveFilename := uploadPath + "/" + randString + "_" + thumbnail + ext
		err := utils.NewImager().Scaling(saveFilename, thumbSaveFilename, width, height)
		if err != nil {
			app.Log.Errorf("make thumbnail image error: %s", err.Error())
			continue
		}

		data["image_"+thumbnail] = imageUrl+ randString + "_" + thumbnail + ext
	}

	appname := string(ctx.Request.Header.Peek("Appname"))
	app.Log.Infof("appname ["+appname+"] upload image %s success", randString+ext)

	origin := string(ctx.Request.Header.Peek("Origin"))
	if (origin != "") && (origin != "null") {
		ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
	}else {
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
	}

	this.jsonSuccess(ctx, "", data)
}

// 下载
func (this *ImageController) Download(ctx *fasthttp.RequestCtx) {

	name := ctx.UserValue("name").(string)
	rootDir := app.Conf.GetString("upload.root_dir")
	dirNameLen := app.Conf.GetInt("upload.dirname_len")

	if !strings.Contains(name, ".") {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.WriteString("404 not found!")
		return
	}

	ext := name[strings.LastIndex(name, "."):]
	if ext == "" {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.WriteString("404 not found!")
		return
	}
	filename := name[:strings.LastIndex(name, ".")]
	n := strings.Index(name, "_")

	if n != -1 {
		filename = filename[:n]
	}

	imagePath := rootDir + utils.StringToPath(filename, dirNameLen)
	imagePath = imagePath + "/" + name

	fasthttp.ServeFile(ctx, imagePath)
}

//处理跨域
func (this *ImageController) CrossDomain(ctx *fasthttp.RequestCtx)  {

	origin := string(ctx.Request.Header.Peek("Origin"))
	if (origin != "") && (origin != "null") {
		ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
	}else {
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
	}
	ctx.Response.Header.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	ctx.Response.Header.Set("Access-Control-Allow-Headers",
		"Accept, Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Token, Appname")
}