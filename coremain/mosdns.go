/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package coremain

import (
	"errors"
	"fmt"
	"io"

	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/safe_close"
	"go.uber.org/zap"
)

type Mosdns struct {
	logger *zap.Logger // non-nil logger.

	// Plugins
	plugins map[string]any
	sc      *safe_close.SafeClose
}

// NewMosdns initializes a mosdns instance and its plugins.
func NewMosdns(cfg *Config) (*Mosdns, error) {
	// Init logger.
	lg, err := mlog.NewLogger(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("failed to init logger: %w", err)
	}

	m := &Mosdns{
		logger:  lg,
		plugins: make(map[string]any),
		sc:      safe_close.NewSafeClose(),
	}

	// Load plugins.

	// Close all plugins on signal.
	// From here, call m.sc.SendCloseSignal() if any plugin failed to load.
	m.sc.Attach(func(done func(), closeSignal <-chan struct{}) {
		go func() {
			defer done()
			<-closeSignal
			m.logger.Info("starting shutdown sequences")
			for tag, p := range m.plugins {
				if closer, _ := p.(io.Closer); closer != nil {
					m.logger.Info("closing plugin", zap.String("tag", tag))
					_ = closer.Close()
				}
			}
			m.logger.Info("all plugins were closed")
		}()
	})

	// Preset plugins
	if err := m.loadPresetPlugins(); err != nil {
		m.sc.SendCloseSignal(err)
		_ = m.sc.WaitClosed()
		return nil, err
	}
	// Plugins from config.
	if err := m.loadPluginsFromCfg(cfg, 0); err != nil {
		m.sc.SendCloseSignal(err)
		_ = m.sc.WaitClosed()
		return nil, err
	}
	m.logger.Info("all plugins are loaded")

	return m, nil
}

// NewTestMosdnsWithPlugins returns a mosdns instance for testing.
func NewTestMosdnsWithPlugins(p map[string]any) *Mosdns {
	return &Mosdns{
		logger:  mlog.Nop(),
		plugins: p,
		sc:      safe_close.NewSafeClose(),
	}
}

func (m *Mosdns) GetSafeClose() *safe_close.SafeClose {
	return m.sc
}

// CloseWithErr is a shortcut for m.sc.SendCloseSignal
func (m *Mosdns) CloseWithErr(err error) {
	m.sc.SendCloseSignal(err)
}

// Logger returns a non-nil logger.
func (m *Mosdns) Logger() *zap.Logger {
	return m.logger
}

// GetPlugin returns a plugin.
func (m *Mosdns) GetPlugin(tag string) any {
	return m.plugins[tag]
}

func (m *Mosdns) loadPresetPlugins() error {
	for tag, f := range LoadNewPersetPluginFuncs() {
		p, err := f(NewBP(tag, m))
		if err != nil {
			return fmt.Errorf("failed to init preset plugin %s, %w", tag, err)
		}
		m.plugins[tag] = p
	}
	return nil
}

// loadPluginsFromCfg loads plugins from this config. It follows include first.
func (m *Mosdns) loadPluginsFromCfg(cfg *Config, includeDepth int) error {
	const maxIncludeDepth = 8
	if includeDepth > maxIncludeDepth {
		return errors.New("maximum include depth reached")
	}
	includeDepth++

	// Follow include first.
	for _, s := range cfg.Include {
		subCfg, path, err := loadConfig(s)
		if err != nil {
			return fmt.Errorf("failed to read config from %s, %w", s, err)
		}
		m.logger.Info("load config", zap.String("file", path))
		if err := m.loadPluginsFromCfg(subCfg, includeDepth); err != nil {
			return fmt.Errorf("failed to load config from %s, %w", s, err)
		}
	}

	for i, pc := range cfg.Plugins {
		if err := m.newPlugin(pc); err != nil {
			return fmt.Errorf("failed to init plugin #%d %s, %w", i, pc.Tag, err)
		}
	}
	return nil
}
