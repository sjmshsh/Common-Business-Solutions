type Params struct {
	ctx    *gin.Context
	chunks int64
	chunk  int64
	size   int64
	name   string
	md5    string
}

func (h *HandlerUser) UploadFile(ctx *gin.Context) {
	result := &project_common.Result{}
	file, head, err := ctx.Request.FormFile("file")
	defer file.Close()
	size := head.Size
	name := head.Filename
	// 用户ID从JWT里面解析
	token := ctx.Request.Header.Get("token")
	parseToken, err := util.ParseToken(token)
	if err != nil {
		log.Println(err)
	}
	userId := parseToken.Uid
	// 任务ID使用雪花算法生成
	worker, err := util.NewWorker(0)
	if err != nil {
		log.Println(err)
	}
	// 任务ID
	id := worker.GetId()
	// md5值也是一样的
	md5 := ctx.Query("md5")
	Schunk := ctx.Query("chunk")
	Schunks := ctx.Query("chunks")
	chunk, _ := strconv.ParseInt(Schunk, 10, 64)
	chunks, _ := strconv.ParseInt(Schunks, 10, 64)
	tempDirPath := common.FilePath + md5
	tempFileName := name + "_tmp"
	os.MkdirAll(tempDirPath, os.ModePerm)
	newFile, err := os.Create(tempDirPath + tempFileName)
	if err != nil {
		log.Println(err)
	}
	defer newFile.Close()
	// 用来计算offset的初始位置
	// 0 = 文件开始位置
	// 1 = 当前位置
	// 2 = 文件结尾处
	var whence = 0
	offset := chunk * common.ChunkSize
	path := tempDirPath + tempFileName

	if err != nil {
		log.Println(err)
	}
	newFile.Seek(offset, whence)
	io.Copy(newFile, file)
	pararm := &Params{
		ctx:    ctx,
		chunks: chunks,
		chunk:  chunk,
		size:   size,
		name:   name,
		md5:    md5,
	}
	isOk := checkAndSetUploadProgress(pararm)
	if isOk {
		renameFile(path, tempDirPath+name)
		// 文件重命名之后马上向服务发送rpc请求，录入数据库
		resp, err := UserServiceClient.UploadFile(context.Background(), &pb.UploadFileRequest{
			UserId: int64(userId),
			Id:     id,
			Size:   size,
			Name:   name,
			Md5:    md5,
			Path:   path,
		})
		if err != nil {
			code, msg := errs.ParseGrpcError(err)
			ctx.JSON(http.StatusOK, result.Fail(code, msg))
			return
		}
		ctx.JSON(http.StatusOK, resp)
	}
}

func renameFile(oldPath string, newPath string) {
	os.Rename(oldPath, newPath)
}

func checkAndSetUploadProgress(param *Params) bool {
	// 把该分段标记成true表示完成
	chunk := param.chunk
	chunks := param.chunks
	md5 := param.md5
	key := common.FileProcessingStatus + md5
	dao.Rdb.SetBit(dao.RCtx, key, chunk, 1)
	flag := true
	// 检查是否全部完成
	for i := 0; i < int(chunks); i++ {
		result, err := dao.Rdb.GetBit(dao.RCtx, key, int64(i)).Result()
		if err != nil {
			log.Println(err)
		}
		if result == 0 {
			flag = false
			break
		}
	}
	if flag == true {
		// 说明文件已经上传完毕了
		dao.Rdb.HSet(dao.RCtx, common.FileUploadStatus, md5, "true")
		return true
	} else {
		// 说明文件还没有上传完毕
		// 如果你不存在这个哈希的key就顺便创建，如果存在的话就直接跳过
		n, err := dao.Rdb.Exists(dao.RCtx, common.FileUploadStatus, md5).Result()
		if err != nil {
			log.Println(err)
		}
		if n <= 0 {
			dao.Rdb.HSet(dao.RCtx, common.FileUploadStatus, md5, "false")
		}
		return false
	}
}
