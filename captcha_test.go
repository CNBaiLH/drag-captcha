/**
* @Author: Lanhai Bai
* @Date: 2021/2/19 15:13
* @Description:
 */
package drag_captcha

import (
	"fmt"
	"github.com/gomodule/redigo/redis"
	"testing"
)

func TestNewDragCaptcha(t *testing.T) {
	r := NewRedisCaptchaStore("tcp","127.0.0.1:6379",redis.DialPassword("123"),redis.DialDatabase(2))
	c,err := NewDragCaptcha(r)
	if err != nil {
		t.Fatal(err)
	}
	b,err := c.CreateImage("dsdsdqww")
	if err != nil {
		t.Fatal(err)
	}

	t.Log(b)

	rt := c.MaskOffset()

	v := c.Valid( "dsdsdqww",float64(rt.Min.X)+5)
	fmt.Println("验证结果:",v)
}