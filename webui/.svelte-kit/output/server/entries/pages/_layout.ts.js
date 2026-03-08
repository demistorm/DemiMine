import { redirect } from "@sveltejs/kit";
function load({ url }) {
  const token = null;
  if (url.pathname !== "/login") {
    throw redirect(307, "/login");
  }
  return {
    token
  };
}
export {
  load
};
