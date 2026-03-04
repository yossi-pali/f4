package main

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	db, err := sql.Open("mysql", "frontend3:eTheem9Iu3aiba@tcp(10.10.227.10:3306)/12go?parseTime=true&loc=UTC")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Q0: Check what province IDs exist for "Surat Thani" in the province table
	fmt.Println("=== Province entries matching 'Surat' ===")
	rows00, err := db.Query(`
		SELECT province_id, name, parent_id FROM province WHERE name LIKE '%Surat%'
	`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		defer rows00.Close()
		for rows00.Next() {
			var provID int
			var name string
			var parentID sql.NullInt64
			rows00.Scan(&provID, &name, &parentID)
			fmt.Printf("  province_id=%d name=%s parent_id=%v\n", provID, name, parentID)
		}
	}

	// Q0b: Check route_place entries with from_place_id involving Surat Thani candidates
	fmt.Println("\n=== route_place entries with from_place_id IN (73, 138) and to_place_id=44 ===")
	rows0b, err := db.Query(`
		SELECT rp.route_place_id, rp.from_place_type, rp.from_place_id, rp.to_place_type, rp.to_place_id
		FROM trip_pool4_route_place rp
		WHERE rp.from_place_id IN (73, 138) AND rp.to_place_id = 44
		   AND rp.from_place_type = 'p' AND rp.to_place_type = 'p'
	`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		defer rows0b.Close()
		for rows0b.Next() {
			var rpID, fromID, toID int
			var fromType, toType string
			rows0b.Scan(&rpID, &fromType, &fromID, &toType, &toID)
			fmt.Printf("  route_place_id=%d  from=%s%d -> to=%s%d\n", rpID, fromType, fromID, toType, toID)
		}
	}

	// Q1: Station pairs for 73p -> 44p (Surat Thani province -> Chiang Mai)
	fmt.Println("\n=== Station pairs for 73p -> 44p (Surat Thani -> Chiang Mai) ===")
	rows0, err := db.Query(`
		SELECT DISTINCT rs.from_id, rs.to_id
		FROM trip_pool4_route_place rp
		JOIN trip_pool4_route_place_station rps ON rps.route_place_id = rp.route_place_id
		JOIN trip_pool4_route_station rs ON rs.route_station_id = rps.route_station_id
		WHERE rp.from_place_type = 'p' AND rp.from_place_id = 73
		  AND rp.to_place_type = 'p' AND rp.to_place_id = 44
		ORDER BY rs.from_id, rs.to_id
	`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	defer rows0.Close()
	for rows0.Next() {
		var fromID, toID int
		rows0.Scan(&fromID, &toID)
		fmt.Printf("  from=%d -> to=%d\n", fromID, toID)
	}

	// Q2: trip_pool4_set data for Surat Thani (73p) -> Chiang Mai (44p) connections
	fmt.Println("\n=== trip_pool4_set for Surat Thani (73p) -> Chiang Mai (44p) ===")
	rows1, err := db.Query(`
		SELECT tps.set_id, tps.pack_id, tps.trip2_day, tps.trip3_day,
		       tps.trip1_key, tps.trip2_key, COALESCE(tps.trip3_key, '') as trip3_key, tps.trip_key
		FROM trip_pool4_set tps
		WHERE tps.set_id IN (
			SELECT DISTINCT tp.set_id
			FROM trip_pool4_route_place rp
			JOIN trip_pool4_route_place_station rps ON rps.route_place_id = rp.route_place_id
			JOIN trip_pool4_route_station rs ON rs.route_station_id = rps.route_station_id
			JOIN trip_pool4 tp ON tp.from_id = rs.from_id AND tp.to_id = rs.to_id
			WHERE rp.from_place_type = 'p' AND rp.from_place_id = 73
			  AND rp.to_place_type = 'p' AND rp.to_place_id = 44
			  AND tp.set_id > 0
		)
		LIMIT 50
	`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	defer rows1.Close()
	count := 0
	for rows1.Next() {
		var setID, packID, trip2Day, trip3Day int
		var trip1Key, trip2Key, trip3Key, tripKey string
		rows1.Scan(&setID, &packID, &trip2Day, &trip3Day, &trip1Key, &trip2Key, &trip3Key, &tripKey)
		fmt.Printf("  set_id=%-6d pack_id=%-6d trip2_day=%-3d trip3_day=%-3d trip1_key=%-35s trip2_key=%-35s trip3_key=%-35s trip_key=%s\n",
			setID, packID, trip2Day, trip3Day, trip1Key, trip2Key, trip3Key, tripKey)
		count++
	}
	fmt.Printf("\nTotal rows: %d\n", count)

	// Q3: Also show the trip_pool4 connection entries themselves
	fmt.Println("\n=== trip_pool4 connection entries for 73p -> 44p (set_id > 0) ===")
	rows2, err := db.Query(`
		SELECT DISTINCT tp.set_id, tp.trip_key, tp.from_id, tp.to_id, tp.operator_id
		FROM trip_pool4_route_place rp
		JOIN trip_pool4_route_place_station rps ON rps.route_place_id = rp.route_place_id
		JOIN trip_pool4_route_station rs ON rs.route_station_id = rps.route_station_id
		JOIN trip_pool4 tp ON tp.from_id = rs.from_id AND tp.to_id = rs.to_id
		WHERE rp.from_place_type = 'p' AND rp.from_place_id = 73
		  AND rp.to_place_type = 'p' AND rp.to_place_id = 44
		  AND tp.set_id > 0
		ORDER BY tp.set_id
		LIMIT 30
	`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var setID, fromID, toID, opID int
		var tripKey string
		rows2.Scan(&setID, &tripKey, &fromID, &toID, &opID)
		fmt.Printf("  set_id=%-6d trip_key=%-40s from=%d -> to=%d  op=%d\n",
			setID, tripKey, fromID, toID, opID)
	}

	// Q4: Also try with 138p in case it exists but has no route_place entries - check directly
	fmt.Println("\n=== Station pairs for 138p -> 44p (alternate Surat Thani ID) ===")
	rows3, err := db.Query(`
		SELECT DISTINCT rs.from_id, rs.to_id
		FROM trip_pool4_route_place rp
		JOIN trip_pool4_route_place_station rps ON rps.route_place_id = rp.route_place_id
		JOIN trip_pool4_route_station rs ON rs.route_station_id = rps.route_station_id
		WHERE rp.from_place_type = 'p' AND rp.from_place_id = 138
		  AND rp.to_place_type = 'p' AND rp.to_place_id = 44
		ORDER BY rs.from_id, rs.to_id
	`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		defer rows3.Close()
		cnt := 0
		for rows3.Next() {
			var fromID, toID int
			rows3.Scan(&fromID, &toID)
			fmt.Printf("  from=%d -> to=%d\n", fromID, toID)
			cnt++
		}
		if cnt == 0 {
			fmt.Println("  (no results)")
		}
	}

	// Q5: Check all route_place entries that go TO place 44 (Chiang Mai) with from place containing 'surat'
	fmt.Println("\n=== All route_place entries going TO place_id=44 ===")
	rows4, err := db.Query(`
		SELECT rp.route_place_id, rp.from_place_type, rp.from_place_id, rp.to_place_type, rp.to_place_id
		FROM trip_pool4_route_place rp
		WHERE rp.to_place_id = 44 AND rp.to_place_type = 'p'
		ORDER BY rp.from_place_id
	`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		defer rows4.Close()
		for rows4.Next() {
			var rpID, fromID, toID int
			var fromType, toType string
			rows4.Scan(&rpID, &fromType, &fromID, &toType, &toID)
			fmt.Printf("  route_place_id=%d  from=%s%d -> to=%s%d\n", rpID, fromType, fromID, toType, toID)
		}
	}
}
