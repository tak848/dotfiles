return {
    -- mise でインストール済みの LSP は mason = false で直接使用
    {
        "neovim/nvim-lspconfig",
        opts = {
            servers = {
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
