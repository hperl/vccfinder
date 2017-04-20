package main

import (
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
)

var pool *redis.Pool
var ErrNil = redis.ErrNil

const (
	RedisInitKey    = "ghprojectInit"
	RedisDoneKey    = "ghprojectDone"
	RedisWorkingKey = "ghprojectWorking"
)

func InitRedis() {
	pool = newPool()
}

func newPool() *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", "131.220.109.52:6386")
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func MarkAsWorking(reponame, tag string) {
	conn := pool.Get()
	defer conn.Close()

	conn.Do("HSET", RedisWorkingKey, reponame, tag)
}

func MarkAsDone(reponame string) {
	conn := pool.Get()
	defer conn.Close()

	conn.Do("HDEL", RedisWorkingKey, reponame)
	conn.Do("RPUSH", RedisDoneKey, reponame)
}

func ReturnRepo(reponame string) {
	conn := pool.Get()
	defer conn.Close()

	conn.Do("HDEL", RedisWorkingKey, reponame)
	conn.Do("RPUSH", RedisInitKey, reponame)
}

func GetNextRepo() (string, error) {
	conn := pool.Get()
	defer conn.Close()

	return redis.String(conn.Do("RPOP", RedisInitKey))
}

func PrintProgress() {
	conn := pool.Get()
	defer conn.Close()

	left, err := conn.Do("LLEN", RedisInitKey)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%3d repositories left\n", left.(int64))

	done, _ := conn.Do("LLEN", RedisDoneKey)
	fmt.Printf("%3d repositories done\n", done.(int64))

	if working, err := redis.Strings(conn.Do("HGETALL", RedisWorkingKey)); err == nil {
		for i := 0; i < len(working); i += 2 {
			fmt.Printf("%v -> %v\n", working[i+1], working[i])
		}
	}
}

func WriteReposToRedis() {
	var rname string

	conn := pool.Get()
	defer conn.Close()

	repos, err := DB.Db.Query("SELECT name FROM repositories WHERE language in ('C', 'C++')")
	if err != nil {
		panic(err)
	}
	defer repos.Close()
	conn.Do("DEL", RedisInitKey)
	conn.Do("DEL", RedisWorkingKey)
	conn.Do("DEL", RedisDoneKey)
	for repos.Next() {
		if err := repos.Scan(&rname); err != nil {
			panic(err)
		}
		conn.Do("RPUSH", RedisInitKey, rname)
	}
	if err := repos.Close(); err != nil {
		panic(err)
	}
}

func Selftest() error {
	conn := pool.Get()
	defer conn.Close()

	_, err := conn.Do("PING")
	return err
}
