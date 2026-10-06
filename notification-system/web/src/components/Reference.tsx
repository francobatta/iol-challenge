import { RedocStandalone } from "redoc"

const font = "'IBM Plex Sans Variable', system-ui, sans-serif"

/**
 * The API's documentation, drawn by Redoc from the OpenAPI description the console
 * serves at /openapi.yaml. scrollOffset is the height of whatever is fixed above it.
 */
export default function Reference({ scrollOffset }: { scrollOffset: number }) {
  return (
    <RedocStandalone
      specUrl="/openapi.yaml"
      options={{
        scrollYOffset: scrollOffset,
        hideDownloadButton: true,
        // The guide is prose and curl; the generated samples would only repeat the reference.
        hideHostname: true,
        theme: {
          colors: { primary: { main: "#171717" } },
          typography: { fontFamily: font, fontSize: "15px", headings: { fontFamily: font, fontWeight: "600" } },
          sidebar: { backgroundColor: "#fafafa" },
        },
      }}
    />
  )
}
