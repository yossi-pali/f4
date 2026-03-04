package main

import (
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/subosito/gotenv"
)

func main() {
	gotenv.Load()
	tpDSN := os.Getenv("DB_TRIPPOOL_TH_DSN")
	tpDB, err := sqlx.Open("mysql", tpDSN)
	if err != nil {
		panic(err)
	}
	defer tpDB.Close()

	defDSN := os.Getenv("DB_DEFAULT_DSN")
	defDB, err := sqlx.Open("mysql", defDSN)
	if err != nil {
		panic(err)
	}
	defer defDB.Close()

	// Focus: composites from 5936→5957 where BOTH legs are bookable
	// Check integration, trip2_day, and departure2_time details
	fmt.Println("=== Bookable composites from 5936→5957 with integration info ===")

	type HeadOp struct {
		TripKey    string `db:"trip_key"`
		OperatorID int    `db:"operator_id"`
		SetID      int    `db:"set_id"`
	}
	var headOps []HeadOp
	tpDB.Select(&headOps, `
		SELECT tp.trip_key, tp.operator_id, tp.set_id
		FROM trip_pool4 tp
		WHERE tp.from_id = 5936 AND tp.to_id = 5957 AND tp.set_id > 0
	`)

	type SetRow struct {
		Trip1Key string `db:"trip1_key"`
		Trip2Key string `db:"trip2_key"`
		Trip2Day int    `db:"trip2_day"`
		PackID   int    `db:"pack_id"`
	}
	type OpInfo struct {
		Bookable int `db:"bookable"`
		SellerID int `db:"seller_id"`
	}
	type SellerInfo struct {
		Bookable int `db:"bookable"`
	}

	for _, ho := range headOps {
		var set SetRow
		err := tpDB.Get(&set, `SELECT trip1_key, trip2_key, trip2_day, COALESCE(pack_id,0) as pack_id FROM trip_pool4_set WHERE set_id = ?`, ho.SetID)
		if err != nil {
			continue
		}
		var t1op, t2op int
		tpDB.Get(&t1op, `SELECT operator_id FROM trip_pool4 WHERE trip_key = ?`, set.Trip1Key)
		tpDB.Get(&t2op, `SELECT operator_id FROM trip_pool4 WHERE trip_key = ?`, set.Trip2Key)
		var t1oi, t2oi OpInfo
		defDB.Get(&t1oi, `SELECT bookable, seller_id FROM operator WHERE operator_id = ?`, t1op)
		defDB.Get(&t2oi, `SELECT bookable, seller_id FROM operator WHERE operator_id = ?`, t2op)
		var t1si, t2si SellerInfo
		defDB.Get(&t1si, `SELECT bookable FROM seller WHERE seller_id = ?`, t1oi.SellerID)
		defDB.Get(&t2si, `SELECT bookable FROM seller WHERE seller_id = ?`, t2oi.SellerID)

		t1Pass := t1oi.Bookable != 0 && t1si.Bookable != 0
		t2Pass := t2oi.Bookable != 0 && t2si.Bookable != 0

		if t1Pass && t2Pass {
			// Get integration for the HEAD trip's operator
			var headIntID int
			defDB.Get(&headIntID, `SELECT COALESCE(MIN(i.integration_id),0) FROM integration i
				JOIN operator o ON o.seller_id = i.seller_id
				WHERE o.operator_id = ?`, ho.OperatorID)

			// Get integration for trip1's operator
			var t1IntID int
			defDB.Get(&t1IntID, `SELECT COALESCE(MIN(i.integration_id),0) FROM integration i
				JOIN operator o ON o.seller_id = i.seller_id
				WHERE o.operator_id = ?`, t1op)

			// Check departure2_time on HEAD price rows for 2026-03-23
			type PriceRow struct {
				Dep  int     `db:"departure_time"`
				Dep2 int     `db:"departure2_time"`
				Price []byte `db:"price"`
			}
			var prices []PriceRow
			tpDB.Select(&prices, `SELECT departure_time, COALESCE(departure2_time,0) as departure2_time, price
				FROM trip_pool4_price WHERE trip_key = ? AND godate = '2026-03-23'`, ho.TripKey)

			hasDep2 := false
			for _, p := range prices {
				if p.Dep2 > 0 {
					hasDep2 = true
					break
				}
			}

			fmt.Printf("  set=%d pack=%d trip2_day=%d head=%s headOp=%d headInt=%d "+
				"t1op=%d(s=%d,int=%d) t2op=%d(s=%d) hasDep2=%v prices=%d\n",
				ho.SetID, set.PackID, set.Trip2Day, ho.TripKey, ho.OperatorID, headIntID,
				t1op, t1oi.SellerID, t1IntID, t2op, t2oi.SellerID, hasDep2, len(prices))
		}
	}

	// Also check: for composites with trip2_day=1 and both bookable,
	// does trip2 have prices for 2026-03-24 (next day)?
	fmt.Println("\n=== Multi-day composites (trip2_day=1) with both bookable - trip2 next-day prices ===")
	for _, ho := range headOps {
		var set SetRow
		err := tpDB.Get(&set, `SELECT trip1_key, trip2_key, trip2_day, COALESCE(pack_id,0) as pack_id FROM trip_pool4_set WHERE set_id = ?`, ho.SetID)
		if err != nil || set.Trip2Day != 1 {
			continue
		}
		var t1op, t2op int
		tpDB.Get(&t1op, `SELECT operator_id FROM trip_pool4 WHERE trip_key = ?`, set.Trip1Key)
		tpDB.Get(&t2op, `SELECT operator_id FROM trip_pool4 WHERE trip_key = ?`, set.Trip2Key)
		var t1oi, t2oi OpInfo
		defDB.Get(&t1oi, `SELECT bookable, seller_id FROM operator WHERE operator_id = ?`, t1op)
		defDB.Get(&t2oi, `SELECT bookable, seller_id FROM operator WHERE operator_id = ?`, t2op)
		var t1si, t2si SellerInfo
		defDB.Get(&t1si, `SELECT bookable FROM seller WHERE seller_id = ?`, t1oi.SellerID)
		defDB.Get(&t2si, `SELECT bookable FROM seller WHERE seller_id = ?`, t2oi.SellerID)

		t1Pass := t1oi.Bookable != 0 && t1si.Bookable != 0
		t2Pass := t2oi.Bookable != 0 && t2si.Bookable != 0
		if !t1Pass || !t2Pass {
			continue
		}

		var headIntID int
		defDB.Get(&headIntID, `SELECT COALESCE(MIN(i.integration_id),0) FROM integration i
			JOIN operator o ON o.seller_id = i.seller_id
			WHERE o.operator_id = ?`, ho.OperatorID)

		// Trip2 prices for next day
		var t2PriceCount int
		tpDB.Get(&t2PriceCount, `SELECT COUNT(*) FROM trip_pool4_price WHERE trip_key = ? AND godate = '2026-03-24'`, set.Trip2Key)

		// Head departure2_time
		type DepInfo struct {
			Dep2 int `db:"dep2"`
		}
		var dep2s []DepInfo
		tpDB.Select(&dep2s, `SELECT COALESCE(departure2_time,0) as dep2
			FROM trip_pool4_price WHERE trip_key = ? AND godate = '2026-03-23'`, ho.TripKey)

		fmt.Printf("  set=%d headInt=%d trip2_day=%d t2key=%s t2op=%d t2prices_0324=%d dep2s=%v\n",
			ho.SetID, headIntID, set.Trip2Day, set.Trip2Key, t2op, t2PriceCount, dep2s)
	}

	fmt.Println("\nDONE")
}
