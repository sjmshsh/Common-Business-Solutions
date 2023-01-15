	// UserCheckIn 签到的key
	UserCheckIn = "usercheckin:"

// Response 通用Response
type Response struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
}

type CheckInRequest struct {
	UserId int64  `json:"userId"`
	Year   string `json:"year"`
	Month  string `json:"month"`
	Day    string `json:"day"`
}

func CheckIn(ctx *gin.Context) {
	// TODO 根据JWT，或者其他的什么东西获得用户的ID，这个得根据你的业务来
	userId := int64(1) // 我们这里就直接随便给一个ID就可以了
	// 获取目前的年份和月份还有天
	year := time.Now().Format("2006")
	month := time.Now().Format("01")
	day := time.Now().Format("02")
	// control层把东西发给service层进行业务逻辑开发
	req := &request.CheckInRequest{
		UserId: userId,
		Year:   year,
		Month:  month,
		Day:    day,
	}
	resp := service.CheckIn(req)
	ctx.JSON(resp.Status, resp.Msg)
}

func CheckIn(request *request.CheckInRequest) *response.Response {
	userId := request.UserId
	year := request.Year
	month := request.Month
	day := request.Day
	d, _ := strconv.ParseInt(day, 10, 64)
	// 组装redis的key
	id := strconv.FormatInt(userId, 10)
	key := redis.UserCheckIn + id
	// 拼装
	// 2023:01:15  2023 01 15
	value := fmt.Sprintf(":%s:%s", year, month)
	key = key + value
	// 签到的代码
	redis2.Rdb.SetBit(redis2.RCtx, key, d-1, 1)
	// 设置过期时间, 30天，可以长一点
	redis2.Rdb.Expire(redis2.RCtx, key, time.Hour*24*30)
	// 缓存层已经设置，接下来使用消息队列异步存储到存储层MySQL
	message := rabbitmq.CheckInMessage{
		UserId: userId,
		Year:   year,
		Month:  month,
		Day:    day,
	}
	mq := rabbitmq.NewRabbitMQTopics("sign", "sign-", "hello")
	mq.PublishTopics(message)
	return &response.Response{
		Status: http.StatusOK,
		Msg:    "用户签到成功",
	}
}

func InitSignConsumer() {
	mq := rabbitmq.NewRabbitMQTopics("sign", "sign-", "hello")
	go mq.ConsumeTopicsCheckIn()
}


	// 签到
	r.GET("/check", api.CheckIn)
	// 查看签到信息
	r.GET("/getSign", api.GetSign)

// Sign 签到
type Sign struct {
	Id     int64  `json:"id"`
	UserId int64  `json:"user_id"`
	Year   string `json:"year"`
	Month  string `json:"month"`
	Day    string `json:"day"`
}

func (Sign) TableName() string {
	return "sign"
}

// User 积分
type User struct {
	Id             int64  `json:"id"`
	UserName       string `json:"userName"`
	PasswordDigest string `json:"passwordDigest"`
	Phone          string `json:"phone"`
	Integral       int    `json:"integral"`
}

func (User) TableName() string {
	return "user"
}

	go func() {
		for delivery := range msgs {
			// 消息逻辑处理，可以自行设计逻辑
			body := delivery.Body
			message := &CheckInMessage{}
			err = json.Unmarshal(body, message)
			if err != nil {
				log.Println(err)
			}
			userId := message.UserId
			year := message.Year
			month := message.Month
			day := message.Day
			worder, _ := util.NewWorker(1)
			id := worder.GetId()
			sign := &model.Sign{
				Id:     id,
				UserId: userId,
				Year:   year,
				Month:  month,
				Day:    day,
			}
			// 插入数据库
			mysql.MysqlDB.Debug().Create(sign)
			// 根据签到信息赠送相应积分
			err = mysql.MysqlDB.Exec("update user set integral = integral + 1 where id = ?", userId).Error
			if err != nil {
				log.Println(err)
			}
			// 为false表示确认当前消息
			delivery.Ack(false)
		}
	}()

type GetSignRequest struct {
	UserId int64 `json:"userId"`
	Year   string
	Month  string
}

type GetSignRequest struct {
	UserId int64 `json:"userId"`
	Year   string
	Month  string
}

func GetSign(request *request.GetSignRequest) *response.Response {
	userId := request.UserId
	id := strconv.FormatInt(userId, 10)
	year := request.Year
	month := request.Month
	// 拼接redis的key
	key := redis.UserCheckIn + id + ":" + year + ":" + month
	fmt.Println(key)
	// 通过bitfield命令返回整个的数组
	// 数组的第一个元素就是一个int64类型的值，我们通过位运算进行操作
	s := fmt.Sprintf("i%d", 31)
	fmt.Println(s)
	result, err := redis2.Rdb.BitField(redis2.RCtx, key, "get", s, 0).Result()
	if err != nil {
		log.Println(err)
	}
	num := result[0]
	fmt.Println(num)
	arr := make([]int64, 31)
	for i := 0; i < 31; i++ {
		// 让这个数字与1做与运算，得到数据的最后一个比特
		if (num & 1) == 0 {
			// 如果为0，说明未签到
			arr[i] = 0

		} else {
			// 如果不为0，说明已经签到了，计数器+1
			arr[i] = 1
		}
		// 把数字右移动一位，抛弃最后一个bit位，继续下一个bit位
		num = num >> 1
	}
	return &response.Response{
		Status: http.StatusOK,
		Msg:    "获取信息成功",
		Data:   arr,
	}
}
