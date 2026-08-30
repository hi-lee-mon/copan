import "@typespec/http";
using Http;

@service(#{ title: "Copan APi" })
namespace Copan;

@route("/healthz")
interface Health {
@get check(): HealthResponse;
}

model HealthResponse {
  version?: string;
  status:"OK";
}
