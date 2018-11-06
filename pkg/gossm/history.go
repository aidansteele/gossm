package gossm

import (
	"database/sql"
	"encoding/json"
	"github.com/aws/aws-sdk-go/service/ssm"
	_ "github.com/mattn/go-sqlite3"
)

type History struct {
	db *sql.DB
}

func NewHistory(path string) (*History, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`create table if not exists commands (commandId text primary key, commandJson text, invocations text, complete bool);`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`create table if not exists invocations (commandId text, instanceId text, stdout text, stderr text, primary key (commandId, instanceId));`)
	if err != nil {
		return nil, err
	}

	return &History{
		db: db,
	}, nil
}

func (h *History) Close() error {
	return h.db.Close()
}

func (h *History) PutCommand(command *ssm.Command, invocations Invocations) error {
	if command != nil {
		bytes, err := json.Marshal(command)
		if err != nil {
			return err
		}
		_, err = h.db.Exec(`insert into commands (commandId, commandJson) values(?, ?) on conflict(commandId) do update set commandJson = excluded.commandJson`, *command.CommandId, bytes)
		if err != nil {
			return err
		}
	}

	if len(invocations) > 0 {
		commandId := ""
		for _, inv := range invocations {
			commandId = *inv.CommandId
		}
		bytes, _ := json.Marshal(invocations)
		_, err := h.db.Exec(`insert into commands (commandId, invocations) values(?, ?) on conflict(commandId) do update set invocations = excluded.invocations`, commandId, bytes)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *History) AppendPayload(msg SsmMessage) error {
	_, err := h.db.Exec(`
		insert into invocations (commandId, instanceId, stdout, stderr) 
		values (?, ?, ?, ?) 
		on conflict (commandId, instanceId) do update set stdout = stdout || excluded.stdout, stderr = stderr || excluded.stderr
	`, msg.CommandId, msg.Payload.InstanceId, msg.Payload.StdoutChunk, msg.Payload.StderrChunk)
	return err
}

type HistoricalCommand struct {
	Command     ssm.Command
	Invocations Invocations
}

func (h *History) Commands() ([]HistoricalCommand, error) {
	rows, err := h.db.Query(`select commandJson, invocations from commands`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []HistoricalCommand

	for rows.Next() {
		var commandJson, invocationsJson []byte
		err = rows.Scan(&commandJson, &invocationsJson)
		if err != nil {
			return nil, err
		}

		command := ssm.Command{}
		err = json.Unmarshal(commandJson, &command)
		if err != nil {
			return nil, err
		}

		invocations := Invocations{}
		err = json.Unmarshal(invocationsJson, &invocations)
		if err != nil {
			return nil, err
		}

		cmd := HistoricalCommand{Command: command, Invocations: invocations}
		commands = append(commands, cmd)
	}

	return commands, nil
}

type HistoricalOutput struct {
	InstanceId string
	Stdout     string
	Stderr     string
}

func (h *History) CommandOutputs(commandId string) ([]HistoricalOutput, error) {
	rows, err := h.db.Query(`select instanceId, stdout, stderr from invocations where commandId = ?`, commandId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outputs []HistoricalOutput

	for rows.Next() {
		output := HistoricalOutput{}
		err = rows.Scan(&output.InstanceId, &output.Stdout, &output.Stderr)
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, output)
	}

	return outputs, nil
}