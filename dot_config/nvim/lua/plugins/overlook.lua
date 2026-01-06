return {
    "WilliamHsieh/overlook.nvim",
    opts = {},
    keys = {
        { "<leader>pd", function() require("overlook.api").peek_definition() end, desc = "Peek definition" },
        { "<leader>pc", function() require("overlook.api").close_all() end, desc = "Close all popup" },
        { "<leader>pu", function() require("overlook.api").restore_popup() end, desc = "Restore popup" },
        { "<leader>ps", function() require("overlook.api").open_in_split() end, desc = "Open in split" },
        { "<leader>pv", function() require("overlook.api").open_in_vsplit() end, desc = "Open in vsplit" },
        { "<leader>po", function() require("overlook.api").open_in_original_window() end, desc = "Open in window" },
    },
}
