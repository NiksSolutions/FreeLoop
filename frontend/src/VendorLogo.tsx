import llamacppOnDark from "./logos/llamacpp-on-dark.svg";
import llamacppOnLight from "./logos/llamacpp-on-light.svg";
import openrouterOnDark from "./logos/openrouter-on-dark.svg";
import openrouterOnLight from "./logos/openrouter-on-light.svg";
import vllm from "./logos/vllm.svg";
import type { Vendor } from "./vendors.ts";

// The vendors' logos for the source marks (GUI-034), only those whose owners' terms let a
// third-party app show them (logos/README.md, RISKS.md L-03). Each is the vendor's file,
// unchanged, drawn as an image, so nothing in it can run or fetch, in the variant made
// for a light or a dark background.
const LOGOS: Partial<Record<Vendor, { onLight: string; onDark: string }>> = {
  llamacpp: { onLight: llamacppOnLight, onDark: llamacppOnDark },
  openrouter: { onLight: openrouterOnLight, onDark: openrouterOnDark },
  vllm: { onLight: vllm, onDark: vllm },
};

export function hasLogo(vendor: Vendor | null | undefined): vendor is Vendor {
  return !!vendor && !!LOGOS[vendor];
}

export default function VendorLogo({ vendor }: { vendor: Vendor }) {
  const logo = LOGOS[vendor];
  if (!logo) return null;
  // Both are there; the style sheet shows the one for the appearance in force.
  return (
    <>
      <img className="vendor-logo vendor-logo-on-light" src={logo.onLight} alt="" draggable={false} />
      <img className="vendor-logo vendor-logo-on-dark" src={logo.onDark} alt="" draggable={false} />
    </>
  );
}
