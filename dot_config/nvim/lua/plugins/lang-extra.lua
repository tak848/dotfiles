return {
    -- mise でインストール済みの LSP は mason = false で直接使用
    -- gd を overlook の peek_definition に置き換え
    {
        "neovim/nvim-lspconfig",
        opts = {
            servers = {
                -- 全 LSP 共通: gd を overlook に置き換え
                ["*"] = {
                    keys = {
                        { "gd", function() require("overlook.api").peek_definition() end, desc = "Goto Definition", has = "definition" },
                    },
                },
                -- mise: go:golang.org/x/tools/gopls
                gopls = { mason = false },
                -- mise: aqua:rust-lang/rust-analyzer
                rust_analyzer = { mason = false },
                -- mise: aqua:tamasfe/taplo
                taplo = { mason = false },
                -- mise: aqua:grafana/jsonnet-language-server
                jsonnet_ls = { mason = false },
            },
        },
    },
    {
        "nvim-treesitter/nvim-treesitter",
        opts = { ensure_installed = { "toml", "jsonnet" } },
    },
}
