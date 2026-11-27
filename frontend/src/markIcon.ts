// A mark's picture (GUI-034), made from an image file the user chose: drawn into at most
// ICON_SIZE pixels a side, its shape kept, and kept as a PNG data: URL, which is what
// config.ValidSourceIcon takes. Drawing it leaves only pixels, whatever the file held,
// so an SVG's markup never reaches the page or config.yaml.

export const ICON_SIZE = 64;

// The most bytes of a file read at all; a picture for a 16-pixel mark needs far fewer.
export const MAX_PICTURE_BYTES = 8 << 20;

export const PICTURE_TYPES = "image/png,image/jpeg,image/webp,image/gif,image/svg+xml";

// fitInside is the size a picture of w by h is drawn at to fit max a side, never enlarged.
export function fitInside(w: number, h: number, max = ICON_SIZE): { width: number; height: number } {
  if (!(w > 0) || !(h > 0)) return { width: max, height: max };
  const k = Math.min(1, max / Math.max(w, h));
  return { width: Math.max(1, Math.round(w * k)), height: Math.max(1, Math.round(h * k)) };
}

// toMarkIcon reads the file and draws it. It throws when the file is too large or is
// not a picture the window can draw.
export async function toMarkIcon(file: Blob): Promise<string> {
  if (file.size > MAX_PICTURE_BYTES) throw new Error("too large");
  const url = URL.createObjectURL(file);
  try {
    const img = new Image();
    img.src = url;
    await img.decode();
    const { width, height } = fitInside(img.naturalWidth, img.naturalHeight);
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const pen = canvas.getContext("2d");
    if (!pen) throw new Error("no canvas");
    pen.drawImage(img, 0, 0, width, height);
    return canvas.toDataURL("image/png");
  } finally {
    URL.revokeObjectURL(url);
  }
}
