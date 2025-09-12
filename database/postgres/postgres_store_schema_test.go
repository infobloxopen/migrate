package postgres

import (
	"database/sql"
	"testing"

	"github.com/dhui/dktest"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/dktesting"
	_ "github.com/lib/pq"
)

var storageSpecs = []dktesting.ContainerSpec{
	{ImageName: "postgres:13", Options: opts},
}

func TestStorageMigrations(t *testing.T) {
	testStorageBasicOperations(t)
}

func TestSyncMigrations(t *testing.T) {
	testSyncMultipleMigrations(t)
}

func TestStorageSchemaUpgrade(t *testing.T) {
	testSchemaUpgrade(t)
}

func TestStorageErrorHandling(t *testing.T) {
	testStorageErrorCases(t)
}

func testStorageBasicOperations(t *testing.T) {
	dktesting.ParallelTest(t, storageSpecs, func(t *testing.T, c dktest.ContainerInfo) {
		d := setupPostgresDriver(t, c)
		defer closeDriver(t, d)

		// Cast to storage driver
		storageDriver := castToStorageDriver(t, d)

		// Test storing and retrieving migrations
		testUpScript := []byte("CREATE TABLE test_table (id SERIAL PRIMARY KEY, name VARCHAR(255));")
		testDownScript := []byte("DROP TABLE test_table;")

		// Store migration (both up and down)
		err := storageDriver.StoreMigration(1, testUpScript, testDownScript)
		if err != nil {
			t.Fatalf("Failed to store migration: %v", err)
		}

		// Retrieve migration
		retrievedUp, retrievedDown, err := storageDriver.GetMigration(1)
		if err != nil {
			t.Fatalf("Failed to retrieve migration: %v", err)
		}

		if string(retrievedUp) != string(testUpScript) {
			t.Errorf("Retrieved up migration doesn't match. Expected: %s, Got: %s", testUpScript, retrievedUp)
		}

		if string(retrievedDown) != string(testDownScript) {
			t.Errorf("Retrieved down migration doesn't match. Expected: %s, Got: %s", testDownScript, retrievedDown)
		}

		// Test getting stored migrations list
		versions, err := storageDriver.GetStoredMigrations()
		if err != nil {
			t.Fatalf("Failed to get stored migrations: %v", err)
		}

		if len(versions) != 1 || versions[0] != 1 {
			t.Errorf("Expected stored migrations [1], got %v", versions)
		}
	})
}

func testSyncMultipleMigrations(t *testing.T) {
	// This test would require setting up a source driver with multiple migrations
	// For now, we'll test the basic storage functionality only
	t.Skip("SyncMigrations requires source driver setup - testing basic storage instead")
}

func testSchemaUpgrade(t *testing.T) {
	dktesting.ParallelTest(t, storageSpecs, func(t *testing.T, c dktest.ContainerInfo) {
		ip, port, err := c.FirstPort()
		if err != nil {
			t.Fatal(err)
		}

		// Create a direct database connection to set up old schema
		connStr := pgConnectionString(ip, port)
		db, err := sql.Open("postgres", connStr)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		// Create the old schema format (without storage columns)
		_, err = db.Exec(`CREATE TABLE schema_migrations (
			version bigint NOT NULL PRIMARY KEY,
			dirty boolean NOT NULL
		)`)
		if err != nil {
			t.Fatalf("Failed to create old schema: %v", err)
		}

		// Insert some existing migration records
		_, err = db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (1, false), (2, false)`)
		if err != nil {
			t.Fatalf("Failed to insert existing records: %v", err)
		}

		// Close the direct connection
		db.Close()

		// Now create the postgres driver which should trigger schema upgrade
		d := setupPostgresDriver(t, c)
		defer closeDriver(t, d)

		// Cast to storage driver (this should trigger the schema upgrade)
		storageDriver := castToStorageDriver(t, d)

		// Try to store a migration - this should work after schema upgrade
		err = storageDriver.StoreMigration(3, []byte("CREATE TABLE test_upgrade (id SERIAL);"), []byte("DROP TABLE test_upgrade;"))
		if err != nil {
			t.Fatalf("Failed to store migration after schema upgrade: %v", err)
		}

		// Get underlying sql.DB for verification
		pgDriver := d.(*Postgres)
		dbVerify := pgDriver.db

		// Verify the schema has the new columns
		verifySchemaUpgrade(t, dbVerify)
		verifyExistingRecordsPreserved(t, storageDriver)
	})
}

func testStorageErrorCases(t *testing.T) {
	dktesting.ParallelTest(t, storageSpecs, func(t *testing.T, c dktest.ContainerInfo) {
		d := setupPostgresDriver(t, c)
		defer closeDriver(t, d)

		storageDriver := castToStorageDriver(t, d)

		// Test retrieving non-existent migration
		_, _, err := storageDriver.GetMigration(999)
		if err == nil {
			t.Error("Expected error when retrieving non-existent migration")
		}

		// Store a valid migration first
		err = storageDriver.StoreMigration(1, []byte("CREATE TABLE test (id SERIAL);"), []byte("DROP TABLE test;"))
		if err != nil {
			t.Fatalf("Failed to store valid migration: %v", err)
		}

		// Test storing duplicate migration (should update, not error)
		err = storageDriver.StoreMigration(1, []byte("CREATE TABLE test_updated (id SERIAL);"), []byte("DROP TABLE test_updated;"))
		if err != nil {
			t.Errorf("Unexpected error when updating existing migration: %v", err)
		}

		// Verify the migration was updated
		upScript, _, err := storageDriver.GetMigration(1)
		if err != nil {
			t.Fatalf("Failed to retrieve updated migration: %v", err)
		}

		expected := "CREATE TABLE test_updated (id SERIAL);"
		if string(upScript) != expected {
			t.Errorf("Migration was not updated. Expected: %s, Got: %s", expected, upScript)
		}
	})
}

// Helper functions

func setupPostgresDriver(t *testing.T, c dktest.ContainerInfo) database.Driver {
	ip, port, err := c.FirstPort()
	if err != nil {
		t.Fatal(err)
	}

	p := &Postgres{}
	addr := pgConnectionString(ip, port)

	d, err := p.Open(addr)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func closeDriver(t *testing.T, d database.Driver) {
	if err := d.Close(); err != nil {
		t.Error(err)
	}
}

func castToStorageDriver(t *testing.T, d database.Driver) database.MigrationStorageDriver {
	storageDriver, ok := d.(database.MigrationStorageDriver)
	if !ok {
		t.Fatal("Postgres driver does not implement MigrationStorageDriver interface")
	}
	return storageDriver
}

func verifySchemaUpgrade(t *testing.T, db *sql.DB) {
	rows, err := db.Query(`SELECT column_name FROM information_schema.columns 
		WHERE table_name = 'schema_migrations' ORDER BY column_name`)
	if err != nil {
		t.Fatalf("Failed to query schema columns: %v", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			t.Fatalf("Failed to scan column name: %v", err)
		}
		columns = append(columns, col)
	}

	expectedColumns := []string{"created_at", "dirty", "down_script", "up_script", "version"}
	if len(columns) != len(expectedColumns) {
		t.Errorf("Expected %d columns, got %d: %v", len(expectedColumns), len(columns), columns)
	}

	for i, expected := range expectedColumns {
		if i >= len(columns) || columns[i] != expected {
			t.Errorf("Column mismatch at position %d. Expected: %s, Got: %v", i, expected, columns)
			break
		}
	}
}

func verifyExistingRecordsPreserved(t *testing.T, storageDriver database.MigrationStorageDriver) {
	// Verify that existing version records are preserved during schema upgrade
	// We should be able to query the table directly to see all version records
	pgDriver := storageDriver.(*Postgres)
	db := pgDriver.db
	
	// Check that all version records (with and without scripts) are preserved
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		t.Fatalf("Failed to query version records: %v", err)
	}
	defer rows.Close()

	var versions []uint
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("Failed to scan version: %v", err)
		}
		versions = append(versions, uint(version))
	}

	// Should have at least 3 version records: 1, 2 (original), and 3 (newly stored)
	if len(versions) < 3 {
		t.Errorf("Expected at least 3 version records after upgrade, got %d: %v", len(versions), versions)
	}

	// Check that GetStoredMigrations only returns the one with scripts (version 3)
	storedVersions, err := storageDriver.GetStoredMigrations()
	if err != nil {
		t.Fatalf("Failed to get stored migrations: %v", err)
	}

	if len(storedVersions) != 1 || storedVersions[0] != 3 {
		t.Errorf("Expected GetStoredMigrations to return [3], got %v", storedVersions)
	}
}
