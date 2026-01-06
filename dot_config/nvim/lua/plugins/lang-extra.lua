return {
    -- TOML (taplo)
    {
        "neovim/nvim-lspconfig",
        opts = {
            servers = {
                taplo = {},
            },
        },
    },
    {
        "nvim-treesitter/nvim-treesitter",
        opts = { ensure_installed = { "toml" } },
    },
    -- jsonnet
    {
        "neovim/nvim-lspconfig",
        opts = {
            servers = {
                jsonnet_ls = {},
            },
        },
    },
    {
        "nvim-treesitter/nvim-treesitter",
        opts = { ensure_installed = { "jsonnet" } },
    },
}
