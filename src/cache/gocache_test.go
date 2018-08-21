package cache_test

import (
	"cache"
	"fmt"
	"sync"
	"testing"
	"time"
)

func GetDataSucc(key interface{}) (interface{}, int) {
	fmt.Println("in get:", key)
	v, ok := key.(string)
	if ok {
		return v + "_value", 0
	} else {
		return "not get", 0
	}
}

func GetDataSlow(key interface{}) (interface{}, int) {
	fmt.Println("in slow get:", key)
	v, ok := key.(string)
	time.Sleep(time.Second)
	if ok {
		return v + "_value", 0
	} else {
		return "not get", 0
	}
}

func TestCache_PutGet(t *testing.T) {
	c := cache.New()
	c.SetExpire(time.Second)            //设置缓存过期时间,可选
	c.SetClearInterval(3 * time.Second) //定时清理过期数据,可选
	c.SetMaxKeyCount(100000)            //设置LRU队列最长大小,可选

	k := "key1"        //key val支持任意类型
	v, ret := c.Get(k) //第一次不存在返回空且false
	fmt.Println(v, ret)
	if ret == 0 {
		t.Error("c.Get err")
	}

	v, _ = GetDataSucc(k)
	c.Put(k, v) //更新缓存

	v, ret = c.Get(k)
	fmt.Println(v, ret)
	if ret != 0 {
		t.Error("c.Put ang Get err")
	}

	time.Sleep(time.Second * 2)

	v, ret = c.Get(k)
	fmt.Println(v, ret) //过期后返回过期值和false，可用于过期后拉取新值时仍然失败的降级
	if v == nil {
		t.Error("c.expire err")
	}

	time.Sleep(time.Second)
	v, ret = c.Get(k)
	if ret == 0 {
		t.Error("c.clearInterval err")
	}
	fmt.Println(v, ret)
}

func TestCache_MulGetFirst(t *testing.T) {
	c := cache.New()
	c.SetExpire(time.Second)
	c.SetUpFunc(GetDataSlow)

	wg := sync.WaitGroup{}
	wg.Add(3)
	//test mul goroutine up differrent key and same key
	go func() {
		v, ret := c.Get("key1")
		if ret != 0 {
			t.Error("TestCache_MulGetFirst test err")
		} else {
			fmt.Println(v)
		}
		wg.Done()
	}()

	go func() {
		v, ret := c.Get("key2")
		if ret != 0 {
			t.Error("TestCache_MulGetFirst test err")
		} else {
			fmt.Println(v)
		}
		wg.Done()
	}()

	go func() {
		v, ret := c.Get("key1")
		if ret != 0 {
			t.Error("TestCache_MulGetFirst test err")
		} else {
			fmt.Println(v)
		}
		wg.Done()
	}()
	wg.Wait()
}

func TestCache_UpFunc(t *testing.T) {
	c := cache.New()
	c.SetExpire(time.Second)
	c.SetUpFunc(GetDataSucc)

	v, ret := c.Get("key1")
	if ret != 0 {
		t.Error("c.Get key1 err in auto upfunc")
	} else {
		fmt.Println(v, ret)
	}

	v, ret = c.Get("key2")
	if ret != 0 {
		t.Error("c.Get key2 err in auto upfunc")
	} else {
		fmt.Println(v, ret)
	}

	//测试多goroutine同时更新
	var wg sync.WaitGroup
	i := 10
	for j := 0; j < i; j++ {
		wg.Add(1)
		go func() {
			v, ret = c.Get("key3")
			if ret != 0 {
				t.Error("c.Get key3 err in auto upfunc")
			} else {
				fmt.Println(v, ret)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	time.Sleep(2 * time.Second)

	//测试过期同时更新
	for j := 0; j < i; j++ {
		wg.Add(1)
		go func() {
			v, ret = c.Get("key1")
			if ret != 0 {
				t.Error("c.Get key1 err in auto upfunc")
			} else {
				fmt.Println(v, ret)
			}
			wg.Done()
		}()
	}
	wg.Wait()

}
