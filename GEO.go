func (u *UserService) Location(ctx context.Context, request *LocationRequest) (*Response, error) {
	longitude := request.Longitude
	o, err := strconv.ParseFloat(longitude, 64)
	if err != nil {
		log.Println(err)
	}
	latitude := request.Latitude
	a, err := strconv.ParseFloat(latitude, 64)
	if err != nil {
		log.Println(err)
	}
	userId := request.UserId
	location := request.Location
	s := strconv.FormatInt(userId, 10)
	key := common.UserLocation + s
	dao.Rdb.GeoAdd(dao.RCtx, key, &redis.GeoLocation{
		Name:      location,
		Longitude: o,
		Latitude:  a,
	})
	return &Response{
		Status: http.StatusOK,
		Msg:    "插入信息成功",
	}, nil
}

func (u *UserService) FindFriend(ctx context.Context, request *FindFriendRequest) (*FindFriendResponse, error) {
	longitude := request.Longitude
	latitude := request.Latitude
	o, err := strconv.ParseFloat(longitude, 64)
	if err != nil {
		log.Println(err)
	}
	a, err := strconv.ParseFloat(latitude, 64)
	if err != nil {
		log.Println(err)
	}
	key := common.UserLocation
	result, err := dao.Rdb.GeoRadius(dao.RCtx, key, o, a, &redis.GeoRadiusQuery{
		Radius:      5,
		Unit:        "km",
		WithCoord:   false, //传入WITHCOORD参数，则返回结果会带上匹配位置的经纬度
		WithDist:    true,  //传入WITHDIST参数，则返回结果会带上匹配位置与给定地理位置的距离。
		WithGeoHash: false, //传入WITHHASH参数，则返回结果会带上匹配位置的hash值
		Sort:        "ASC", //默认结果是未排序的，传入ASC为从近到远排序，传入DESC为从远到近排序。
	}).Result()
	if err != nil {
		log.Println(err)
	}
	N := len(result)
	name := make([]string, N)
	dist := make([]float32, N)
	for i := 0; i < N; i++ {
		name[i] = result[i].Name
		dist[i] = float32(result[i].Dist)
	}
	return &FindFriendResponse{
		Name: name,
		Dist: dist,
	}, nil
}
