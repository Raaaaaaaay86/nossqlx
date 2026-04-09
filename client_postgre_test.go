package nossqlx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PostgreClientTestSuite struct {
	suite.Suite
	client *PostgreClient
	config ClientConfig
}

func TestPostgreClientTestSuite(t *testing.T) {
	suite.Run(t, new(PostgreClientTestSuite))
}

// SetupSuite prepares table `test_table` for testing
func (s *PostgreClientTestSuite) SetupSuite() {
	s.config = ClientConfig{
		Host:       "localhost",
		Port:       5432,
		Database:   "postgres",
		Username:   "root",
		Password:   "root",
		SQLTimeout: 5 * time.Second,
	}

	client, err := NewSqlxPostgreClient(s.config)
	s.Require().NoError(err)
	s.client = client

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = s.client.Pool().Exec(ctx, `CREATE TABLE IF NOT EXISTS test_table (id SERIAL PRIMARY KEY, name TEXT)`)
	s.Require().NoError(err)
}

// SetupTest cleans up table before each test
func (s *PostgreClientTestSuite) SetupTest() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.client.Pool().Exec(ctx, `TRUNCATE TABLE test_table RESTART IDENTITY`)
	s.Require().NoError(err)
}

// TestConstructor make sure `NewSqlxPostgreClient` able to establish PostgreSQL connection
func (s *PostgreClientTestSuite) TestConstructor() {
	client, err := NewSqlxPostgreClient(s.config)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), client)
	assert.NotNil(s.T(), client.Pool())
}

// TestPool make sure Pool() can get *pgxpool.Pool and do a Ping() to database successfully
func (s *PostgreClientTestSuite) TestPool() {
	pool := s.client.Pool()
	assert.NotNil(s.T(), pool)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := pool.Ping(ctx)
	assert.NoError(s.T(), err)
}

// TestSession make sure to acquire SQL execution session
func (s *PostgreClientTestSuite) TestSession() {
	ctx, cancel, runner, err := s.client.Session(context.Background())
	assert.NoError(s.T(), err)
	defer cancel()

	_, err = runner.Exec(ctx, "INSERT INTO test_table (name) VALUES ($1)", "test_session")
	assert.NoError(s.T(), err)

	var name string
	err = runner.QueryRow(ctx, "SELECT name FROM test_table WHERE name = $1", "test_session").Scan(&name)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "test_session", name)
}

// TestTransaction makes sure nested and non-nested transaction both can be handled correctly
func (s *PostgreClientTestSuite) TestTransaction() {
	t := s.T()

	truncate := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := s.client.Pool().Exec(ctx, `TRUNCATE TABLE test_table RESTART IDENTITY`)
		s.Require().NoError(err)
	}

	t.Run("Non Nested Transaction", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			innerCtx, cancel, runner, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer cancel()

			_, err = runner.Exec(innerCtx, "INSERT INTO test_table (name) VALUES ($1)", "non-nested")
			return err
		})
		assert.NoError(t, err)

		// Verify data exists
		var count int
		err = s.client.Pool().QueryRow(context.Background(), "SELECT COUNT(*) FROM test_table WHERE name = $1", "non-nested").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("Non Nested Transaction Rollback", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			innerCtx, cancel, runner, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer cancel()

			_, err = runner.Exec(innerCtx, "INSERT INTO test_table (name) VALUES ($1)", "rollback")
			if err != nil {
				return err
			}
			return assert.AnError // trigger rollback
		})
		assert.Error(t, err)

		// Verify data does not exist
		var count int
		err = s.client.Pool().QueryRow(context.Background(), "SELECT COUNT(*) FROM test_table WHERE name = $1", "rollback").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("Nested Transaction", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			// First Level
			innerCtx1, cancel1, runner1, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer cancel1()

			_, err = runner1.Exec(innerCtx1, "INSERT INTO test_table (name) VALUES ($1)", "nested-1")
			if err != nil {
				return err
			}

			return BeginTx(ctx, func(ctx context.Context) error {
				// Second Level
				innerCtx2, cancel2, runner2, err := s.client.Session(ctx)
				if err != nil {
					return err
				}
				defer cancel2()

				_, err = runner2.Exec(innerCtx2, "INSERT INTO test_table (name) VALUES ($1)", "nested-2")
				return err
			})
		})
		assert.NoError(t, err)

		// Verify both exist
		var count int
		err = s.client.Pool().QueryRow(context.Background(), "SELECT COUNT(*) FROM test_table").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("Nested Transaction Rollback (layer-1 and layer-2 both rollback)", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			// First Level
			innerCtx1, cancel1, runner1, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer cancel1()

			_, err = runner1.Exec(innerCtx1, "INSERT INTO test_table (name) VALUES ($1)", "nested-rollback-1")
			if err != nil {
				return err
			}

			return BeginTx(ctx, func(ctx context.Context) error {
				return assert.AnError
			})
		})
		assert.Error(t, err)

		var count int
		err = s.client.Pool().QueryRow(context.Background(), "SELECT COUNT(*) FROM test_table WHERE name LIKE 'nested-rollback-%'").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("Nested Transaction Rollback (layer-1 commit and layer-2 rollback)", func(t *testing.T) {
		truncate()
		err := BeginTx(context.Background(), func(ctx context.Context) error {
			// First Level
			innerCtx1, cancel1, runner1, err := s.client.Session(ctx)
			if err != nil {
				return err
			}
			defer cancel1()

			_, err = runner1.Exec(innerCtx1, "INSERT INTO test_table (name) VALUES ($1)", "nested-rollback-1")
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
		err = s.client.Pool().QueryRow(context.Background(), "SELECT COUNT(*) FROM test_table WHERE name LIKE 'nested-rollback-%'").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}
