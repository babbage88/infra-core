package deployment

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	_ "github.com/lib/pq"
)

const defaultAppSchemaName string = "public"

// need to add AppSchemaName as field, have it default to "public" when not defined.
type PgSetupScriptsRequest struct {
	DbHostName        string `json:"dbHostname"`
	SuperuserUsername string `json:"superuserUsername"`
	SuperuserPassword string `json:"superuserPassword"`
	DatabaseName      string `json:"databaseName"`
	ServiceUsername   string `json:"serviceUsername"`
	ServicePassword   string `json:"servicePassword"`
	DbPort            int32  `json:"dbPort"`
	SchemaName        string `json:"schemaName"`
}

// Checks whether originalValue is an empty or blank string.
//
// if string is blank/empty it will be set to the defaultValue.
// Returns originalValue if its not blank or empty.
func validateStringWithDefaultValue(originalValue string, defaultValue string) string {
	if len(strings.TrimSpace(originalValue)) == 0 {
		return defaultValue
	}

	return originalValue
}

func GenerateDbUserScriptsHandler() func(w http.ResponseWriter, r *http.Request) {
	return generateDbUserScriptsHandler
}

// Example Go Handler (with a simple HTTP route)
func generateDbUserScriptsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		// Preflight request
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusOK)
		return
	}
	// Extract the parameters from the request (e.g., JSON payload)
	var requestData PgSetupScriptsRequest

	// Parse the JSON request
	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// statically setting the schema name to the one I use for the dev deb, need to come back and make it a parameter whose value defaults to public
	requestData.SchemaName = "public"

	sqlScripts := GenerateSqlScript(requestData.DbHostName,
		requestData.SuperuserUsername,
		requestData.SuperuserPassword,
		requestData.ServiceUsername,
		requestData.ServicePassword,
		requestData.DatabaseName,
		requestData.DbPort,
		requestData.SchemaName,
	)

	// Send the script back to the frontend
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sqlScripts)
}

type PgDevDbSetupScriptsResponse struct {
	ShellScript      string `json:"create_dev_db.sh"`
	PgCreateScript   string `json:"pg_create_db.sql"`
	AppDbSetupScript string `json:"pg_app_db.sql"`
}

func GenerateSqlScript(dbHostname, superUsername, superUserPassword, appUsername, appPass, appDbName string, dbPort int32, schemaName string) PgDevDbSetupScriptsResponse {
	schemaName = validateStringWithDefaultValue(schemaName, defaultAppSchemaName)
	var sqlScript strings.Builder
	var appSqlScript strings.Builder
	var shellScript strings.Builder
	pgDb := "postgres"

	pgSqlScriptName := "pg_create_db.sql"
	appDbScriptName := "pg_app_db.sql"

	shellScript.WriteString("#!/bin/sh\n")

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", dbHostname, dbPort, superUsername, superUserPassword, pgDb)
	appDBConn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", dbHostname, dbPort, superUsername, superUserPassword, appDbName)

	shellScript.WriteString(fmt.Sprintf("psql -Atx \"%s\" -f %s\n", connStr, pgSqlScriptName))
	shellScript.WriteString(fmt.Sprintf("psql -Atx \"%s\" -f %s\n", appDBConn, appDbScriptName))

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		slog.Warn("error connecting to database, assuming db and user do not exist for script generation", slog.String("error", err.Error()))
	}
	defer db.Close()

	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", appDbName).Scan(&exists)
	if err != nil {
		slog.Warn("error checking if db exists, assuming it does not exist", slog.String("error", err.Error()))
	}

	if !exists {
		sqlScript.WriteString("/* #### Creating new database #### */")
		sqlScript.WriteByte('\n')
		sqlScript.WriteString(fmt.Sprintf(`CREATE DATABASE %s WITH OWNER = postgres ENCODING = 'UTF8' TEMPLATE = template0;`, appDbName))
		sqlScript.WriteByte('\n')
	}

	var userExists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", appUsername).Scan(&userExists)
	if err != nil {
		slog.Warn("error checking if user exists, assuming it does not", slog.String("error", err.Error()))
	}

	if userExists {
		sqlScript.WriteString(fmt.Sprintf(`ALTER USER %s WITH PASSWORD '%s';`, appUsername, appPass))
		sqlScript.WriteByte('\n')
	} else {
		sqlScript.WriteString(fmt.Sprintf(`CREATE ROLE %s WITH LOGIN;`, appUsername))
		sqlScript.WriteByte('\n')
		sqlScript.WriteString(fmt.Sprintf(`ALTER USER %s WITH PASSWORD '%s';`, appUsername, appPass))
		sqlScript.WriteByte('\n')
	}

	sqlScript.WriteString(fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE %s TO %s;`, appDbName, appUsername))
	sqlScript.WriteByte('\n')

	appdb, err := sql.Open("postgres", appDBConn)
	if err != nil {
		slog.Warn("failed to connect to target database", slog.String("error", err.Error()))
	}
	defer appdb.Close()

	// defining raw string literal query up here first, so the tab formatting is less distracting when defining the appSqlStatemest slice
	alterDefaultPrivsQry := fmt.Sprintf(`
ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA %s
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %s;`, appUsername, schemaName, appUsername)

	appSqlStatements := []string{
		// Allow user to look up objects in schema
		fmt.Sprintf(`GRANT USAGE ON SCHEMA %s TO %s;`, schemaName, appUsername),
		// Allows user to "look up" objects in the schema
		fmt.Sprintf(`GRANT ALL ON SCHEMA %s TO %s;`, schemaName, appUsername),
		// Set the appUsername value as owner of the application schema
		fmt.Sprintf(`ALTER SCHEMA %s OWNER TO %s;`, schemaName, appUsername),
		// Ensure that the appUsername will have proper privs on all new tables in the schema
		alterDefaultPrivsQry,
		// Grant all privs to existing tables
		fmt.Sprintf(`GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA %s TO %s;`, schemaName, appUsername),
		// Grant on sequences for tables with auto-incrementing IDs (e.g., serial or IDENTITY columns)
		fmt.Sprintf(`GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA %s TO %s;`, schemaName, appUsername),
	}

	appSqlScript.WriteString("/* ######### SQL Statements to execute while connected to the new application database ########## */\n")
	for _, sqlStatement := range appSqlStatements {
		appSqlScript.WriteString(sqlStatement)
		appSqlScript.WriteByte('\n')
	}

	sqlScript.WriteString("/* ######### SQL Statements to execute while connected to the default postgres database ########## */\n")
	sqlScript.WriteString(fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE %s TO %s;`, appDbName, appUsername))
	sqlScript.WriteByte('\n')

	return PgDevDbSetupScriptsResponse{
		ShellScript:      shellScript.String(),
		PgCreateScript:   sqlScript.String(),
		AppDbSetupScript: appSqlScript.String(),
	}
}

type PgDevDbExecStatement struct {
	QryCmd []string `json:"qryCmds"`
	DbName string   `json:"dbName"`
	DbUser string   `json:"dbUser"`
}

type PgCreateDevDbAndUserResponse struct {
	StatemensExecuted []PgDevDbExecStatement `json:"pdDevDbDeploymentStatements"`
	Errors            []error                `json:"deploymentErrors"`
}
