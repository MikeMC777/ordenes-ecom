# Use environment variables with fallback to local values
$PRODUCT_BASE = if ($env:PRODUCT_SERVICE_BASEURL) { $env:PRODUCT_SERVICE_BASEURL } else { "http://product:8081" }
$LOCAL_PRODUCT_BASE = if ($env:LOCAL_PRODUCT_SERVICE_BASEURL) { $env:LOCAL_PRODUCT_SERVICE_BASEURL } else { "http://localhost:8081" }
$ORDER_BASE   = if ($env:ORDER_SERVICE_BASEURL)   { $env:ORDER_SERVICE_BASEURL }   else { "http://localhost:8082" }
$USER_ADDR    = if ($env:USER_SERVICE_ADDR)    { $env:USER_SERVICE_ADDR }    else { "localhost:50051" }

# Normalize (without trailing slash)
$PRODUCT_BASE = $PRODUCT_BASE.TrimEnd('/')
$ORDER_BASE   = $ORDER_BASE.TrimEnd('/')

Write-Host “== Seeds: User + Order ==”

# 1) Create seed user (ignore error if already exists)
$userReq = '{"username":"seed11","email":"seed11@test.com","password":"123456"}'
try {
  $userRaw = (& grpcurl -plaintext -d $userReq $USER_ADDR user.v1.UserService/CreateUser) | Out-String
  $user = $userRaw | ConvertFrom-Json
  $userId = $user.user.id
  Write-Host "User created: $userId"
} catch {
  Write-Warning "Possible duplicate user; searching for email seed@test.com"
  $userId = Read-Host "Manually enter the user_id of the seed (or press Enter to skip creating an order)"
}

# 2) Take one product (the first one on the list)
try {
  $list = irm "$LOCAL_PRODUCT_BASE/products?limit=1&offset=0"
  if (-not $list.items -or $list.items.Count -eq 0) { throw "There are no products." }
  $prodId = $list.items[0].id
  Write-Host "Selected product: $prodId ($($list.items[0].name))"
} catch {
  Write-Error "No products available. Did you run the product seed migration?"
  exit 1
}

# 3) Create order if we have userId
if ($userId) {
  $orderBody = @"
{
  "user_id": "$userId",
  "items": [
    { "product_id": "$prodId", "quantity": 2 }
  ]
}
"@
  try {
    Write-Host "Order Body: $orderBody"
    $orderRes = (curl.exe -s -X POST "$ORDER_BASE/orders" -H "Content-Type: application/json" --data-binary $orderBody) | ConvertFrom-Json
    $orderId = $orderRes.order.id
    Write-Host "Order Res: $orderRes"
    Write-Host "Order created: $orderId"
  } catch {
    Write-Warning "Unable to create order (check services)."
  }
} else {
  Write-Host "No user_id; order creation was skipped."
}

Write-Host "`n== Summary =="
Write-Host "User (seed@test.com): $userId"
Write-Host "Product: $prodId"
if ($orderId) { Write-Host "Order: $orderId" }
