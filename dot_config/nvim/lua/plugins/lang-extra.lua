return {
    -- mise でインストール済みの LSP は mason = false で直接使用
    -- gd を overlook の peek_definition に置き換え
    {
        "neovim/nvim-lspconfig",
        opts = function(_, opts)
            opts.servers = opts.servers or {}
            -- mise: go:golang.org/x/tools/gopls
            opts.servers.gopls = { mason = false }
            -- mise: aqua:rust-lang/rust-analyzer
            opts.servers.rust_analyzer = { mason = false }
            -- mise: aqua:tamasfe/taplo
            opts.servers.taplo = { mason = false }
            -- mise: aqua:grafana/jsonnet-language-server
            opts.servers.jsonnet_ls = { mason = false }

            -- LSP キーマップで gd を overlook に置き換え
            local keys = require("lazyvim.plugins.lsp.keymaps").get()
            keys[#keys + 1] = {
                "gd",
                function()
                    require("overlook.api").peek_definition()
                end,
                desc = "Goto Definition",
                has = "definition",
            }
        end,
    },
    {
        "nvim-treesitter/nvim-treesitter",
        opts = { ensure_installed = { "toml", "jsonnet" } },
    },
}
