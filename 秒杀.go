package service

import (
	"context"
	"github.com/go-redis/redis/v8"
	redis2 "github.com/sjmshsh/IM/db/redis"
	"github.com/sjmshsh/IM/rabbitmq"
	"github.com/sjmshsh/IM/request"
	"github.com/sjmshsh/IM/response"
	"log"
	"net/http"
)

func Seckill(request *request.SeckillRequest) *response.Response {
	// !!!!!!前提是我们已经做好了缓存预热，所以我们这里就直接把缓存写到redis里面去了
	// 1. 获取用户ID
	userId := request.UserId
	// 2. 获取产品ID
	productId := request.ProductId
	// 3. 执行Lua脚本
	var seckill = redis.NewScript(`
local productId = ARGV[1]
local userId = ARGV[2]
local stockKey = KEYS[1]..productId
local orderKey = KEYS[2]..productId
local res = redis.call('get', stockKey)
if (tonumber(res, 10) <= 0) then
    return 1
end
redis.call('incrby', stockKey, -1)
redis.call('sadd', orderKey, userId)
return 0
`)
	ctx := context.Background()
	keys := []string{"seckill:stock:", "seckill:order:"}
	values := []interface{}{productId, userId}
	resInterface, err := seckill.Run(ctx, redis2.Rdb, keys, values...).Result()
	res := resInterface.(int64)
	if err != nil {
		log.Println(err)
		log.Println("出现了严重的错误！！！")
	}
	if res == 1 {
		return &response.Response{
			Status: http.StatusOK,
			Msg:    "用户秒杀失败，库存不足",
		}
	}
	if res == 2 {
		return &response.Response{
			Status: http.StatusOK,
			Msg:    "用户秒杀失败，您已经秒杀过了，不可用继续秒杀",
		}
	}
	// 到这里就说明秒杀成功了
	// 这个时候我们可以进行异步下单
	mq := rabbitmq.NewRabbitMQTopics("seckill", "seckill-order")
	mq.PublishTopics(rabbitmq.Message{
		UserId:    userId,
		ProductId: productId,
	})
	return &response.Response{
		Status: http.StatusOK,
		Msg:    "用户秒杀成功",
	}
}

func InitSeckillConsumer() {
	mq := rabbitmq.NewRabbitMQTopics("seckill", "seckill-order")
	go mq.ConsumeTopics()
}

package api

import (
	"github.com/gin-gonic/gin"
	"github.com/sjmshsh/IM/request"
	"github.com/sjmshsh/IM/service"
	"log"
	"strconv"
)

// Seckill /seckill?id=100
func Seckill(ctx *gin.Context) {
	idString := ctx.Query("id")
	id, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		log.Println(err)
	}
	req := &request.SeckillRequest{
		ProductId: id,
		UserId:    1,
	}
	log.Println(req)
	response := service.Seckill(req)
	ctx.JSON(response.Status, response.Msg)
}

package rabbitmq

import (
	"encoding/json"
	"fmt"
	"github.com/sjmshsh/IM/db/mysql"
	"github.com/sjmshsh/IM/model"
	"github.com/sjmshsh/IM/pkg/util"
	"github.com/streadway/amqp"
	"gorm.io/gorm"
	"log"
)

// 连接信息amqp://用户名:密码@ip/Virtual Hosts
const rmqURL = "amqp://guest:guest@127.0.0.1:5672/lxy"

// Rabbit RabbitMQ结构体
type Rabbit struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	QueueName string // 队列名称
	Exchange  string // 交换机名称
	Key       string // bind Key 名称
	MqUrl     string // 连接信息
}

// NewRabbitMQ 创建Rabbit结构体实例
func NewRabbitMQ(queueName, exchange, key string) *Rabbit {
	return &Rabbit{
		QueueName: queueName,
		Exchange:  exchange,
		Key:       key,
		MqUrl:     rmqURL,
	}
}

// Destroy 断开channel和connection
func (r Rabbit) Destroy() error {
	err := r.channel.Close()
	err = r.conn.Close()
	return err
}

// 错误处理函数
func (r Rabbit) failOnErr(err error, msg string) {
	if err != nil {
		log.Fatal(msg, err)
	}
}

// NewRabbitMQTopics 创建Topics模式下RabbitMQ实例
func NewRabbitMQTopics(exchangeName, routingKey string) *Rabbit {
	rabbitMQ := NewRabbitMQ("order_service", exchangeName, routingKey) // 创建RabbitMQ实例
	var err error
	rabbitMQ.conn, err = amqp.Dial(rabbitMQ.MqUrl) // 获取connection
	rabbitMQ.failOnErr(err, "failed to connect rabbitmq!")
	rabbitMQ.channel, err = rabbitMQ.conn.Channel() // 获取channel
	rabbitMQ.failOnErr(err, "failed to open a channel")
	return rabbitMQ
}

// PublishTopics Topics模式 生产者
func (r Rabbit) PublishTopics(msg any) {
	// 1.尝试创建交换机
	err := r.channel.ExchangeDeclare(
		r.Exchange, // 交换机名字
		"topic",    // 交换机类型，这里使用topic类型，即: Topics模式
		true,       // 是否持久化
		false,      // 是否自动删除
		false,      // true表示这个exchange不可以被client用来推送消息，仅用来进行exchange和exchange之间的绑定
		false,      // 是否阻塞处理
		nil,        // 额外的属性
	)
	r.failOnErr(err, "Failed to declare an exchange")
	// 2.发送消息
	bytes, err := json.Marshal(msg)
	if err != nil {
		log.Println(err)
	}
	err = r.channel.Publish(
		r.Exchange,
		r.Key, // Topics模式这里要指定key
		false, // 如果为true，根据自身exchange类型和routekey规则无法找到符合条件的队列会把消息返还给发送者
		false, // 如果为true，当exchange发送消息到队列后发现队列上没有消费者，则会把消息返还给发送者
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        bytes,
		},
	)
	if err != nil {
		log.Println(err)
	}
}

// ConsumeTopics Topics模式 消费者
func (r Rabbit) ConsumeTopics() {
	// 1.试探性创建交换机
	err := r.channel.ExchangeDeclare(
		r.Exchange, // 交换机名字
		"topic",    // 交换机类型，这里使用topic类型，即: Topics模式
		true,       // 是否持久化
		false,      // 是否自动删除
		false,      // true表示这个exchange不可以被client用来推送消息，仅用来进行exchange和exchange之间的绑定
		false,      // 是否阻塞处理
		nil,        // 额外的属性
	)
	r.failOnErr(err, "Failed to declare an exchange")
	// 2.试探性创建队列，这里注意队列名称不要写
	queue, err := r.channel.QueueDeclare(
		"order_service", // 随机生产队列名称
		false,           // 是否持久化
		false,           // 是否自动删除
		true,            // 是否具有排他性
		false,           // 是否阻塞处理
		nil,             // 额外的属性
	)
	r.failOnErr(err, "Failed to declare a queue")
	// 3.绑定队列到exchange中
	err = r.channel.QueueBind(
		queue.Name, // 队列名
		r.Key,      // 路由参数，如果匹配消息发送的时候指定的路由参数，消息就投递到当前队列（在Topics模式下，这里的key要指定）
		r.Exchange, // 交换机名字，需要跟消息发送端定义的交换器保持一致
		false,      // 是否阻塞处理
		nil,        // 额外的属性
	)
	// 4.消费消息
	msgs, err := r.channel.Consume(
		queue.Name, // 队列名称
		"",         // 用来区分多个消费者
		false,      // 是否自动应答
		false,      // 是否独有
		false,      // 设置为true，表示不能将同一个Connection中生产者发送的消息传递给这个Connection中的消费者
		false,      // 队列是否阻塞
		nil,        // 额外的属性
	)
	r.failOnErr(err, "Failed to Consume")
	// 5.启用协程处理消息
	forever := make(chan bool) // 开个channel阻塞住，让开启的协程能一直跑着
	go func() {
		for delivery := range msgs {
			// 消息逻辑处理，可以自行设计逻辑
			body := delivery.Body
			message := &Message{}
			err = json.Unmarshal(body, message)
			if err != nil {
				log.Println(err)
			}
			userId := message.UserId
			productId := message.ProductId
			worder, _ := util.NewWorker(1)
			id := worder.GetId()
			order := &model.Order{
				ID:        id,
				UserID:    userId,
				ProductId: productId,
			}
			// 插入订单
			mysql.MysqlDB.Debug().Create(order)
			// 扣除商品数量
			mysql.MysqlDB.Debug().Model(&model.Product{ID: productId}).UpdateColumn("stock", gorm.Expr("stock - ?", 1))
			// 为false表示确认当前消息
			err = delivery.Ack(false)
			if err != nil {
				log.Println(err)
			}
		}
	}()
	fmt.Println(" [*] Waiting for messages.")
	<-forever
}
