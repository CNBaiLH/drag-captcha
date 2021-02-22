/**
* @Author: Lanhai Bai
* @Date: 2021/2/20 16:21
* @Description:
 */
package main

import (
	drag_captcha "github.com/CNBaiLH/drag-captcha"
	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"
	"image/png"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	r := gin.New()

	dir := getWebRoot()
	r.Static("/static",dir+"/")
	r.LoadHTMLFiles(dir+"/index.html")
	r.GET("/",Index)
	r.GET("/captcha", GetCaptcha)
	r.POST("/login", Login)

	if err := r.Run(":8888"); err != nil {
		log.Fatalln(err)
	}

}

func getWebRoot() string {
	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	return dir
}

func Index(ctx *gin.Context){
	clientId := getNonceStr()
	ctx.HTML(http.StatusOK, "index.html", gin.H{"clientId":clientId})
}

//简略生成客户端ID
func getNonceStr() (nonceStr string) {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	var r = rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 32; i++ {
		idx := r.Intn(len(chars) - 1)
		nonceStr += chars[idx : idx+1]
	}
	return nonceStr
}
//获取验证码
func GetCaptcha(ctx *gin.Context) {
	clientId := ctx.DefaultQuery("client_id","")
	//redis连接
	r := drag_captcha.NewRedisCaptchaStore("tcp","127.0.0.1:6379",redis.DialPassword("123"),redis.DialDatabase(2))
	c,err := drag_captcha.NewDragCaptcha(r)
	if err != nil {
		ctx.JSON(http.StatusOK,gin.H{"code":1,"msg":"获取验证码失败","client_id":clientId})
		return
	}

	imageRGBA,err := c.CreateImage(clientId)
	if err != nil {
		log.Println(err)
		ctx.JSON(http.StatusOK,gin.H{"code":2,"msg":"生成验证码失败","client_id":clientId})
		return
	}
	png.Encode(ctx.Writer,imageRGBA)
}

//登录
func Login(ctx *gin.Context) {
	requestMap := make(map[string]interface{}) //注意该结构接受的内容
	_ = ctx.BindJSON(&requestMap)
	clientId,ok := requestMap["client_id"].(string)
	if !ok {
		ctx.JSON(http.StatusOK,gin.H{"code":3,"msg":"客户端ID不能为空"})
		return
	}
	//拖动的像素
	tnr := requestMap["tn_r"].(float64)
	if tnr <= 0 {
		ctx.JSON(http.StatusOK,gin.H{"code":4,"msg":"请正确滑动验证码"})
		return
	}
	//redis连接
	r := drag_captcha.NewRedisCaptchaStore("tcp","127.0.0.1:6379",redis.DialPassword("123"),redis.DialDatabase(2))
	c,err := drag_captcha.NewDragCaptcha(r)
	if err != nil {
		ctx.JSON(http.StatusOK,gin.H{"code":5,"msg":"服务端出错,请稍后再试"})
		return
	}
	if !c.Valid(clientId,tnr) {
		ctx.JSON(http.StatusOK,gin.H{"code":6,"msg":"验证码验证失败"})
		return
	}
	//dosomething 成功后分配token等一些列业务逻辑
	ctx.JSON(http.StatusOK,gin.H{"code":0,"msg":"验证成功"})
}
