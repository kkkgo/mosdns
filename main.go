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

package main

import (
	"fmt"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/mlog"
	_ "github.com/IrineSistiana/mosdns/v5/plugin"
	"github.com/spf13/cobra"
)

var (
	version = "kkkgo/mosdns:240822.1"
)

func init() {
	coremain.AddSubCmd(&cobra.Command{
		Use:   "version",
		Short: "Print out version info and exit.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	})
	coremain.AddSubCmd(&cobra.Command{
		Use:   "AddMod",
		Short: "AddMod for mosdns.yaml",
		Run: func(cmd *cobra.Command, args []string) {
			coremain.AddMod()
		},
	})
	coremain.AddSubCmd(&cobra.Command{
		Use:   "curl",
		Short: "Curl url filename[s] socks[no]",
		Run: func(cmd *cobra.Command, args []string) {
			coremain.Curl(args)
		},
	})
	coremain.AddSubCmd(&cobra.Command{
		Use:   "eat",
		Short: "eat list",
		Run: func(cmd *cobra.Command, args []string) {
			coremain.Eatlist(args)
		},
	})
}

func main() {
	if err := coremain.Run(); err != nil {
		mlog.S().Fatal(err)
	}
}
