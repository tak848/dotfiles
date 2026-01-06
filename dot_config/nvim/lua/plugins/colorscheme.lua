return {
    {
        "catppuccin/nvim",
        name = "catppuccin",
        opts = {
            flavour = "mocha",
            integrations = {
                flash = true,
                gitsigns = true,
                indent_blankline = { enabled = true },
                lsp_trouble = true,
                mason = true,
                mini = true,
                neotree = true,
                noice = true,
                notify = true,
                telescope = true,
                treesitter_context = true,
                which_key = true,
            },
        },
    },
    {
        "LazyVim/LazyVim",
        opts = { colorscheme = "catppuccin" },
    },
}
