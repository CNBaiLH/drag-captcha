/**
* @Author: ITWorker
* @Date: 2021/2/19 13:36
* @Description:拖动验证码
 */
package drag_captcha

import (
	"encoding/json"
	"errors"
	"github.com/gomodule/redigo/redis"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

var (
	captchaKeyIsTooShortError  = errors.New("primary key is too short")
	captchaNotFoundBasePicture = errors.New("not found base picture")
	captchaNotFoundMaskPicture = errors.New("not found mask picture")

	captchaRuntimePath string
)

type Captcha struct {
	base       string          //底图路径 传入相对路径
	mask       string          //蒙板路径 传入相对路径
	maskOffset image.Rectangle //蒙板偏移量
	deviation  int             //允许误差像素
	lifetime   time.Duration   //有效时间,单位秒
	store      CaptchaStore    //验证码信息存储对象
	baseImage image.Image      //背景图
	dstImageRGBA  *image.RGBA 		//生成后的图，含一个背景图，一个蒙板，一个背景图+蒙板
	maskImage image.Image		//蒙板
	maskImageRGBA *image.RGBA   //主图对应的蒙板区域图像
}

type CaptchaStore interface {
	Set(key string, offset image.Rectangle, lifetime time.Duration)
	Get(key string) *image.Rectangle
	Del(key string)
}

func NewRedisCaptchaStore(network, address string, opts ...redis.DialOption) *redisCaptchaStore {
	pool := &redis.Pool{
		Dial: func() (conn redis.Conn, e error) {
			conn, e = redis.Dial(network, address, opts...)
			if e != nil {
				log.Println("NewRedisCaptchaStore Dial error :", e)
				return nil, e
			}
			return conn, nil
		},
		MaxIdle:         5,
		MaxActive:       10,
		IdleTimeout:     30 * time.Second,
		Wait:            false,
		MaxConnLifetime: 240 * time.Second,
	}
	return &redisCaptchaStore{pool: pool}
}

type redisCaptchaStore struct {
	pool *redis.Pool
}

type cacheRectangle struct {
	X1, Y1 int //左上角的点
	X2, Y2 int //右下角的点
}

func (r *redisCaptchaStore) Set(key string, offset image.Rectangle, lifetime time.Duration) {
	area := cacheRectangle{
		X1: offset.Min.X,
		Y1: offset.Min.Y,
		X2: offset.Max.X,
		Y2: offset.Max.Y,
	}
	areaRt, _ := json.Marshal(area)
	if _, e := r.pool.Get().Do("SET", "drag_captcha:"+key, string(areaRt), "EX", lifetime.Seconds()); e != nil {
		log.Printf("redisCaptchaStore Set key %s error : %v\r\n", key, e)
	}
}
func (r redisCaptchaStore) Get(key string) *image.Rectangle {
	reply, err := redis.Bytes(r.pool.Get().Do("GET", "drag_captcha:"+key))
	if err != nil {
		log.Println("redisCaptchaStore get error: ", err)
		return nil
	}
	area := &cacheRectangle{}
	defer r.Del(key)
	if err = json.Unmarshal(reply, area); err != nil {
		log.Println("redisCaptchaStore get json.Unmarshal error : ", err)
		return nil
	}
	return &image.Rectangle{
		Min: image.Point{X: area.X1, Y: area.Y1},
		Max: image.Point{X: area.X2, Y: area.Y2},
	}
}
func (r redisCaptchaStore) Del(key string) {
	_, _ = r.pool.Get().Do("DEL", "drag_captcha:"+key)
}

//设置底图的存放路径
func WithDragCaptchaBase(basePath string) func(o *Captcha) {
	return func(o *Captcha) {
		o.base = basePath
	}
}

//设置蒙板的存放路径
func WithDragCaptchaMask(maskPath string) func(o *Captcha) {
	return func(o *Captcha) {
		o.mask = maskPath
	}
}

//设置允许误差
func WithDragCaptchaDeviation(deviation int) func(o *Captcha) {
	return func(o *Captcha) {
		if deviation >= 0 {
			o.deviation = deviation
		}
	}
}

//生成指定范围的随机数
func randomIntNumber(start int, end int) int {
	rand.Seed(time.Now().UnixNano())
	return start + rand.Intn(end-start)
}

//获取程序运行的目录
func processRuntimeDir() (string, error) {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	return dir, err
	//return // strings.Replace(dir, "\\", "/", -1)
}
func NewDragCaptcha(store CaptchaStore) (*Captcha, error) {
	if reflect.ValueOf(store).IsNil() {
		return nil,errors.New("NewDragCaptcha CaptchaStore is nil")
	}
	var err error
	captchaRuntimePath, err = processRuntimeDir()
	if err != nil {
		return nil, err
	}
	captcha := &Captcha{
		base: "base.png",
		mask: "mask.png",
		maskOffset: image.Rectangle{
			Min: image.Point{0, 0},
			Max: image.Point{48, 47},
		},
		deviation: 5,
		lifetime:  60 * time.Second,
		store:     store,
	}
	return captcha, nil
}

func (c *Captcha) randMaskOffset(baseX, baseY, maskX, maskY int) (xStart,yStart int) {
	xStart = randomIntNumber(0, baseX-maskX)
	yStart = randomIntNumber(0, baseY-maskY)
	c.maskOffset = image.Rect(xStart, yStart, xStart+maskX, yStart+maskY)
	return
}

func (c *Captcha) MaskOffset() image.Rectangle {
	return c.maskOffset
}

//生成主图
//第一张及最后一张
func (c *Captcha) drawBaseImage() (err error) {
	base := captchaRuntimePath + string(os.PathSeparator) + c.base
	f, err := os.Open(base)
	if err != nil {
		return err
	}
	baseImage, _, err := image.Decode(f)
	if err != nil {
		return  err
	}
	newRectangle := image.Rectangle{
		Min: baseImage.Bounds().Min,
		Max: image.Point{baseImage.Bounds().Max.X, baseImage.Bounds().Max.Y * 3},
	}
	dstImageRGBA := image.NewRGBA(newRectangle)
	//3倍大的主图
	draw.Draw(dstImageRGBA, dstImageRGBA.Bounds(), baseImage, image.Point{0, 0}, draw.Src)

	newRectangle = image.Rectangle{
		Min: image.Point{0, baseImage.Bounds().Max.Y * 2},
		Max: image.Point{baseImage.Bounds().Max.X, baseImage.Bounds().Max.Y * 3},
	}
	//画最后一张图
	draw.Draw(dstImageRGBA, newRectangle, baseImage, image.Point{0, 0}, draw.Src)
	c.baseImage = baseImage
	c.dstImageRGBA = dstImageRGBA
	return nil
}

//生成蒙板，并将蒙板写到第二个区域中
func (c *Captcha) drawMaskImage() (err error) {
	mask := captchaRuntimePath + string(os.PathSeparator) + c.mask
	m, err := os.Open(mask)
	if err != nil {
		return  err
	}
	maskImage, err := png.Decode(m)
	if err != nil {
		return  err
	}

	maskImgRGBA := image.NewRGBA(maskImage.Bounds())
	draw.Draw(maskImgRGBA, maskImage.Bounds(), maskImage, image.Point{0, 0}, draw.Src)

	xStart,yStart := c.randMaskOffset(c.baseImage.Bounds().Size().X,c.baseImage.Bounds().Size().Y,maskImage.Bounds().Size().X,maskImage.Bounds().Size().Y)

	//将主图上面的位置画到mask上面
	draw.Draw(maskImgRGBA, maskImage.Bounds(), c.baseImage, image.Point{xStart, yStart}, draw.Over)

	maskRectangle := image.Rectangle{
		Min: image.Point{0, c.maskOffset.Min.Y + c.baseImage.Bounds().Size().Y},
		Max: image.Point{c.maskOffset.Max.X, c.maskOffset.Max.Y + c.baseImage.Bounds().Size().Y},
	}
	for x := 0; x < maskImgRGBA.Bounds().Size().X; x++ {
		for y := 0; y < maskImgRGBA.Bounds().Size().Y; y++ {
			r, g, b, a := maskImage.At(x, y).RGBA()
			if r == 0 && g == 0 && b == 0 && a == 0 {
				maskImgRGBA.Set(x, y, color.Alpha{0})
			}
		}
	}
	draw.Draw(c.dstImageRGBA, maskRectangle, maskImgRGBA, image.Point{0, 0}, draw.Src)

	c.maskImage = maskImage
	c.maskImageRGBA = maskImgRGBA
	return nil
}

//在第一张图的响应区域画上蒙板
func (c *Captcha) drawMaskOnDstImage() {
	maskRectangle := image.Rectangle{
		Min: image.Point{c.maskOffset.Min.X, c.maskOffset.Min.Y},
		Max: image.Point{c.maskOffset.Max.X, c.maskOffset.Max.Y + c.baseImage.Bounds().Size().Y},
	}
	sp := image.Point{0, 0}
	mp := image.Point{0, 0}
	draw.DrawMask(c.dstImageRGBA, maskRectangle, c.maskImage, sp, c.maskImage, mp, draw.Over)
}

//生成验证码，并存储
//返回文件流
//在指定图片上面画蒙板
/*
	dst  绘图的背景图。
	r 是背景图的绘图区域
	src 是要绘制的图
	sp 是 src 对应的绘图开始点（绘制的大小 r变量定义了）
	mask 是绘图时用的蒙版，控制替换图片的方式。
	mp 是绘图时蒙版开始点（绘制的大小 r变量定义了）
*/
func (c *Captcha) CreateImage(key string,fns ...func(o *Captcha)) (dstImage *image.RGBA, err error) {
	if len(key) < 6 {
		return nil, captchaKeyIsTooShortError
	}
	for _, fn := range fns {
		fn(c)
	}

	//判断底图是否存在，其他状态的底图也返回不存在
	if _, err := os.Stat(captchaRuntimePath + string(os.PathSeparator) + c.base); err != nil {
		if os.IsNotExist(err) {
			return nil, captchaNotFoundBasePicture
		}
		return nil, err
	}

	//判断蒙板是否存在，其他状态的蒙板也返回不存在
	if _, err := os.Stat(captchaRuntimePath + string(os.PathSeparator) + c.mask); err != nil {
		if os.IsNotExist(err) {
			return nil, captchaNotFoundMaskPicture
		}
		return nil, err
	}

	//1step
	if err := c.drawBaseImage(); err != nil {
		return nil,err
	}
	//2step
	if err := c.drawMaskImage(); err != nil {
		return nil, err
	}
	//3step
	c.drawMaskOnDstImage()
	c.store.Set(key, c.maskOffset, c.lifetime)
	//dstBuffer := bytes.NewBuffer(nil)
	//dstBuffer1, _ := os.Create("test.png")
	//png.Encode(dstBuffer1, c.dstImageRGBA)

	//if err = png.Encode(dstBuffer, c.dstImageRGBA); err != nil {
	//	return nil, err
	//}
	return c.dstImageRGBA, nil
}

//验证验证码
//tn 为水平方向拖动的像素
func (c *Captcha) Valid(key string, tn float64) bool {
	offset := c.store.Get(key)
	log.Println("取得验证码数据为:", offset)
	if offset == nil {
		return false
	}
	defer c.store.Del(key)
	if tn < float64(offset.Min.X)-float64(c.deviation) || tn > float64(offset.Min.X)+float64(c.deviation) {
		return false
	}
	return true
}
