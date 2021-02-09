#!/bin/sh
case $1 in
  create1) # db1 db2 db3
    r=1
    primary=$2
    while [ $# -gt 1 ] ; do
      echo "CREATE TABLE $2.clickhouse_go (
          dateTime DateTime64,
          rowIndex UInt64
        ) ENGINE =
        ReplicatedMergeTree('/clickhouse/tables/$2/clickhouse_go', '$r')
        ORDER BY dateTime
        PARTITION BY toYYYYMM(dateTime)" | clickhouse-client || exit $?
      r=`expr $r + 1`
      shift
    done
    ;;

  create2) # db1 (primary)
    echo "ALTER TABLE system.query_log DELETE WHERE 1" |
      clickhouse-client || exit $?
    echo "CREATE TABLE default.clickhouse_go_dist AS $2.clickhouse_go
      ENGINE = Distributed(clickhouse_cluster, '', clickhouse_go)" |
      clickhouse-client || exit $?
    ;;

  drop) # db1 db2 db3
    echo "DROP TABLE IF EXISTS default.clickhouse_go_dist" | clickhouse-client
    while [ $# -gt 1 ] ; do
      echo "DROP TABLE IF EXISTS $2.clickhouse_go" | clickhouse-client
      shift
    done
    ;;

  verify) # N P I R
    NPI=`expr $2 \* $3 \* $4`
    R=$5
    lua > /tmp/inserts_expected.txt <<LUA
      function line(r)
        fmt = '%Y-%m-%d %H:%M:%S'
        sec = math.floor(r / 1000)
        msec = r % 1000
        print(string.format('%s.%03d', os.date(fmt, sec), msec), r)
      end
      for r = 1, 3 do
        print($NPI * $R)
        for x = 1, $NPI do
          istart = (x - 1) * $R
          for j = 1, 5 do
            line(istart + j - 1)
          end
          for j = 1, 5 do
            line(istart + $R - j)
          end
        end
      end
LUA
    ( for i in 1 2 3 ; do # go multiple times to exercise replica selection
        echo "SELECT COUNT(*) FROM default.clickhouse_go_dist;"
        for x in `seq $NPI`; do
          istart=`echo "($x - 1) * $R" | bc`
          for direction in ASC DESC ; do
            echo "SELECT dateTime, rowIndex FROM default.clickhouse_go_dist
              WHERE rowIndex >= $istart
              AND rowIndex < `expr $istart + $R`
              ORDER BY dateTime $direction LIMIT 5;"
          done
        done
      done ) | clickhouse-client -n > /tmp/inserts_actual.txt
    diff -q /tmp/inserts_expected.txt /tmp/inserts_actual.txt
    ;;

esac
