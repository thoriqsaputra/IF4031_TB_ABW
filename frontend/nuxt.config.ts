export default defineNuxtConfig({
  css: ["quill/dist/quill.snow.css", "~/assets/main.css"],
  app: {
    head: {
      title: "Agarthan Reports",
      meta: [
        {
          name: "description",
          content: "Agarthan reports portal for public and personal findings.",
        },
      ],
    },
  },
});
