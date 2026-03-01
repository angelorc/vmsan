export default defineAppConfig({
  docus: {
    name: "vmsan",
    description: "Firecracker microVM sandbox toolkit",
    url: "https://vmsan.dev",
    socials: {
      github: "angelorc/vmsan",
    },
  },
  github: {
    rootDir: "docs",
  },
  ui: {
    colors: {
      primary: "emerald",
      neutral: "zinc",
    },
    commandPalette: {
      slots: {
        item: "items-center",
        input: "[&_.iconify]:size-4 [&_.iconify]:mx-0.5",
        itemLeadingIcon: "size-4 mx-0.5",
      },
    },
    contentNavigation: {
      slots: {
        linkLeadingIcon: "size-4 mr-1",
        linkTrailing: "hidden",
      },
      defaultVariants: {
        variant: "link",
      },
    },
    pageLinks: {
      slots: {
        linkLeadingIcon: "size-4",
        linkLabelExternalIcon: "size-2.5",
      },
    },
  },
});
