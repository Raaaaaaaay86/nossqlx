package nossqlx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type MySQLClientTestSuite struct {
	suite.Suite
	client *MySQLClient
	config ClientConfig
}

func TestMySQLClientTestSuite(t *testing.T) {
	suite.Run(t, new(MySQLClientTestSuite))
}

// SetupSuite prepares table `test_table` for testing
func (s *MySQLClientTestSuite) SetupSuite() {
	// 1. Initial connection to MySQL server using system 'mysql' database
	baseConfig := ClientConfig{
		Host:       "localhost",
		Port:       3306,
		Database:   "mysql",
		Username:   "root",
		Password:   "root",
		SQLTimeout: 5 * time.Second,
	}

	tempClient, err := NewSqlxMySQLClient(baseConfig)
	if err != nil {
		s.T().Skip("MySQL not available, skipping integration tests: ", err)
		return
	}

	// 2. Create a dedicated test database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = tempClient.DB().ExecContext(ctx, `CREATE DATABASE IF NOT EXISTS nossqlx_integration_test`)
	s.Require().NoError(err)
	tempClient.DB().Close()

	// 3. Connect to the dedicated test database
	s.config = baseConfig
	s.config.Database = "nossqlx_integration_test"
	s.client, err = NewSqlxMySQLClient(s.config)
	s.Require().NoError(err)

	// 4. Create table for testing
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = s.client.DB().ExecContext(ctx, `CREATE TABLE IF NOT EXISTS test_table (id INT AUTO_INCREMENT PRIMARY KEY, name TEXT)`)
	s.Require().NoError(err)
}

// SetupTest cleans up table before each test
func (s *MySQLClientTestSuite) SetupTest() {
	if s.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.client.DB().ExecContext(ctx, `TRUNCATE TABLE test_table`)
	s.Require().NoError(err)
}

// TestConstructor make sure `NewSqlxMySQLClient` able to establish MySQLSQL connection
func (s *MySQLClientTestSuite) TestConstructor() {
	if s.client == nil {
		s.T().Skip("MySQL not available")
	}
	client, err := NewSqlxMySQLClient(s.config)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), client)
	assert.NotNil(s.T(), client.DB())
}

// TestSession make sure to acquire SQL execution session
func (s *MySQLClientTestSuite) TestSession() {
	if s.client == nil {
		s.T().Skip("MySQL not available")
	}
	ctx, cancel, runner, release, err := s.client.Session(context.Background())
	assert.NoError(s.T(), err)
	defer cancel()
	defer release()

	_, err = runner.ExecContext(ctx, "INSERT INTO test_table (name) VALUES (?)", "test_session")
	assert.NoError(s.T(), err)

	var name string
	err = runner.QueryRowContext(ctx, "SELECT name FROM test_table WHERE name = ?", "test_session").Scan(&name)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "test_session", name)
}

// TestTransaction makes sure nested and non-nested transaction both can be handled correctly
func (s *MySQLClientTestSuite) TestTransaction() {
	if s.client == nil {
		s.T().Skip("MySQL not available")
	}
	t := s.T()

	truncate := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := s.client.DB().ExecContext(ctx, `TRUNCATE TABLE test_table`)
		s.Require().NoError(err)
	}

	t.Run("Non Nested Transaction", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			innerCtx, _, runner, release, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer release()

			_, err = runner.ExecContext(innerCtx, "INSERT INTO test_table (name) VALUES (?)", "non-nested")
			return err
		})
		assert.NoError(t, err)

		// Verify data exists
		var count int
		err = s.client.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test_table WHERE name = ?", "non-nested").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("Non Nested Transaction Rollback", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			innerCtx, _, runner, release, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer release()

			_, err = runner.ExecContext(innerCtx, "INSERT INTO test_table (name) VALUES (?)", "rollback")
			if err != nil {
				return err
			}
			return assert.AnError // trigger rollback
		})
		assert.Error(t, err)

		// Verify data does not exist
		var count int
		err = s.client.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test_table WHERE name = ?", "rollback").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("Nested Transaction", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			// First Level
			innerCtx1, _, runner1, release1, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer release1()

			_, err = runner1.ExecContext(innerCtx1, "INSERT INTO test_table (name) VALUES (?)", "nested-1")
			if err != nil {
				return err
			}

			return BeginTx(ctx, func(ctx context.Context) error {
				// Second Level
				innerCtx2, _, runner2, release2, err := s.client.Session(ctx)
				if err != nil {
					return err
				}
				defer release2()

				_, err = runner2.ExecContext(innerCtx2, "INSERT INTO test_table (name) VALUES (?)", "nested-2")
				return err
			})
		})
		assert.NoError(t, err)

		// Verify both exist
		var count int
		err = s.client.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test_table").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("Nested Transaction Rollback (layer-1 and layer-2 both rollback)", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			// First Level
			innerCtx1, _, runner1, release1, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer release1()

			_, err = runner1.ExecContext(innerCtx1, "INSERT INTO test_table (name) VALUES (?)", "nested-rollback-1")
			if err != nil {
				return err
			}

			return BeginTx(ctx, func(ctx context.Context) error {
				return assert.AnError
			})
		})
		assert.Error(t, err)

		var count int
		err = s.client.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test_table WHERE name LIKE 'nested-rollback-%'").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("Nested Transaction Rollback (layer-1 commit and layer-2 rollback)", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			// First Level
			innerCtx1, _, runner1, release1, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer release1()

			_, err = runner1.ExecContext(innerCtx1, "INSERT INTO test_table (name) VALUES (?)", "nested-rollback-1")
			if err != nil {
				return err
			}

			if err := BeginTx(ctx, func(ctx context.Context) error {
				return assert.AnError // trigger rollback for all
			}); err != nil {
				assert.Error(t, err)
			}

			return nil
		})
		assert.NoError(t, err)

		// Verify none exist
		var count int
		err = s.client.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test_table WHERE name LIKE 'nested-rollback-%'").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}
