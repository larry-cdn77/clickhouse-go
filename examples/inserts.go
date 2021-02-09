/*
 * High throughput inserting into a sharded set of replicated MergeTree tables
 *
 * The example is intended to work with multiple shard databases per host
 * (node), what is sometimes described as cross-segmented topology. Use case is
 * a host that has one shard's primary replica and other shard's secondary
 * replicas. With the same table name used throughout, the replicas reside in
 * different name databases on each host. Distributed table then specifies an
 * empty database name and ClickHouse supplies a name based on choice of
 * replica.
 *
 * Usage:
 *
 *   scp inserts.sh $host1: ; scp inserts.sh $host2 ; scp inserts.sh $host3
 *   ssh $host1 sh inserts.sh create1 db1 db2
 *   ssh $host2 sh inserts.sh create1 db2 db3
 *   ssh $host3 sh inserts.sh create1 db3 db1
 *   ssh $host1 sh inserts.sh create2 db1
 *   ( go run inserts.go $host1 db1 1 16 10 100000 &
 *     go run inserts.go $host2 db2 2 16 10 100000 &
 *     go run inserts.go $host3 db3 3 16 10 100000 &
 *     wait )
 *   ssh $host1 verify 3 16 10 100000 && echo PASS
 *   ssh $host1 sh inserts.sh drop db1 db2
 *   ssh $host2 sh inserts.sh drop db2 db3
 *   ssh $host3 sh inserts.sh drop db3 db1
 *
 * In the above, $host1, $host2 and $host3 are all hosts of cluster defined
 * in remote_server section of config.xml under the name:
 *
 *   clickhouse_cluster
 *
 * Shards 1, 2 and 3 are configured with databases db1, db2 and db3,
 * respectively. The first database listed with each host corresponds to the
 * primary replica 1, followed by databases for one or more further replicas.
 *
 * Replicas are defined in ZooKeeper with path prefix:
 *
 *   /clickhouse/tables/
 *
 * Table creation consists of two steps so all MergeTree tables are up before
 * distributed table is created. Note that distributed table is put on one node
 * only. Similarly for the drop operation.
 *
 * Run command line arguments after host name are primary database on given node
 * to insert into (eg db1), node number (so row IDs can be unique within the
 * entire test run) and three parameters:
 *
 * P ... concurrency (processes each doing I inserts of size R), eg 16
 * I ... repeat count (inserts), eg 10
 * R ... insert size (rows), eg 100000
 *
 * Table verification runs on the node where distributed table is. It executes
 * queries to look at top 5 and bottom 5 rows corresponding to each insert
 * (total number of inserts is N x P x I).
 *
 */
package main

import (
	"database/sql/driver"
	"fmt"
	"github.com/ClickHouse/clickhouse-go"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	host := os.Args[1]
	database := os.Args[2]
	n, err := strconv.Atoi(os.Args[3])
	n -= 1; // provided 1-based
	if err != nil {
		panic(err)
	}
	P, err := strconv.Atoi(os.Args[4])
	if err != nil {
		panic(err)
	}
	I, err := strconv.Atoi(os.Args[5])
	if err != nil {
		panic(err)
	}
	R, err := strconv.Atoi(os.Args[6])
	if err != nil {
		panic(err)
	}
	barrier := new(sync.WaitGroup)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	options := strings.Join([]string{
		"write_timeout=300",
		"read_timeout=300",
		"compress=true",
		"block_size=10000000",
		"timeout=100",
	}, "&")
	for p := 0; p < P; p++ {
		barrier.Add(1)
		url := fmt.Sprintf("tcp://%s:9000?%s", host, options)
		query := fmt.Sprintf("INSERT INTO %s.clickhouse_go VALUES",
			database) // syntax after "VALUES" unused
		go routine(barrier, sig, url, query, n, P, I, R, p)
	}
	barrier.Wait()
}

func routine(barrier *sync.WaitGroup, sig chan os.Signal, url string,
	query string, nodeNum int, numRoutines int, repeatCount int,
	insertSize int, p int) {
	defer barrier.Done()
	handle, err := clickhouse.Open(url)
	if err != nil {
		panic(err)
	}
	defer handle.Close()
	for c := 0; c < repeatCount; c++ {
		select {
			case <-sig:
				return
			default:
				break
		}
		transaction, err := handle.Begin()
		if err != nil {
			panic(err)
		}
		statement, err := handle.Prepare(query)
		for r := 0; r < insertSize; r++ {
			processStart := (p + nodeNum * numRoutines) * insertSize * repeatCount
			index := processStart + insertSize * c + r
			dateTime := time.Unix(int64(index) / 1000,
				(int64(index) % 1000) * 1000000)
			statement.Exec([]driver.Value{dateTime, index})
		}
		err = transaction.Commit()
		if err != nil {
			panic(err)
		}
	}
}
