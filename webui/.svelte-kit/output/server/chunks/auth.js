import { w as writable } from "./index.js";
const token = writable(
  typeof localStorage !== "undefined" ? localStorage.getItem("token") : null
);
token.subscribe((value) => {
  if (typeof localStorage !== "undefined") {
    if (value) {
      localStorage.setItem("token", value);
    } else {
      localStorage.removeItem("token");
    }
  }
});
export {
  token as t
};
