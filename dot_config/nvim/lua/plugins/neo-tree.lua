return {
  "nvim-neo-tree/neo-tree.nvim",
  opts = {
    filesystem = {
      filtered_items = {
        visible = true, -- gitignore/dotfilesを薄く表示（VSCode風）
        hide_dotfiles = false,
        hide_gitignored = false,
      },
    },
    default_component_configs = {
      icon = {
        use_filtered_colors = true, -- filtered itemsに異なる色を適用
      },
      name = {
        use_filtered_colors = true,
        use_git_status_colors = true,
      },
    },
  },
}
