package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

type doneMsg struct {
	err error
}

type spinModel struct {
	spinner  spinner.Model
	title    string
	err      error
	finished bool
}

func (m spinModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m spinModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case doneMsg:
		m.err = msg.err
		m.finished = true
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.err = huh.ErrUserAborted
			m.finished = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m spinModel) View() string {
	if m.finished {
		return ""
	}
	return m.spinner.View() + " " + m.title
}

func withSpinner(ctx context.Context, title string, fn func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m := spinModel{
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		title:   title,
	}
	program := tea.NewProgram(m, tea.WithContext(ctx))

	errCh := make(chan error, 1)
	go func() {
		errCh <- fn(ctx)
	}()
	go func() {
		err := <-errCh
		program.Send(doneMsg{err: err})
	}()

	final, err := program.Run()
	cancel()
	if err != nil {
		return err
	}
	finished, ok := final.(spinModel)
	if !ok || !finished.finished {
		return huh.ErrUserAborted
	}
	return finished.err
}
