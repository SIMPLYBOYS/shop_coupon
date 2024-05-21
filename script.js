import http from "k6/http";
import { check, sleep } from "k6";

export let options = {
  vus: 200, // Number of concurrent virtual users, adjust as needed
  iterations: 200, // Number of iterations per virtual user
};

const batchSize = 200; // Number of requests to send in each batch, adjust as needed
const numUsers = 40000; // Number of users

// Generate an array of all user IDs
let allUserIds = Array.from({ length: numUsers }, (_, i) => i + 1);

// Shuffle user IDs and split them into batches
let batches = [];
allUserIds = allUserIds.sort(() => 0.5 - Math.random());
for (let i = 0; i < allUserIds.length; i += batchSize) {
  batches.push(allUserIds.slice(i, i + batchSize));
}

export default function () {
  const baseUrl = "http://localhost:8080"; // Server address, modify as needed
  const vuId = __VU; // Get the current virtual user ID
  console.log(`VU ${vuId} is sending requests...`);

  // Send /user requests
  sendRequestsInBatches(batches[vuId], `${baseUrl}/user`, createUserPayload);

  // Send /reserve requests
  sendRequestsInBatches(
    batches[vuId],
    `${baseUrl}/reserve`,
    createReservePayload
  );

  // Send /grab requests
  sendRequestsInBatches(batches[vuId], `${baseUrl}/grab`, createGrabPayload);
}

// Function to send requests in batches, optionally with random order
function sendRequestsInBatches(userIds, url, payloadFunc) {
  // Check if userIds is a non-empty array
  if (!Array.isArray(userIds) || userIds.length === 0) {
    return;
  }

  userIds.forEach((userId) => {
    let payload = payloadFunc(userId);
    let params = {
      headers: {
        "Content-Type": "application/json",
      },
    };
    let res = http.post(url, payload, params);

    // Check if the response status code is 200
    check(res, {
      "status was 200": (r) => r.status === 200,
    });

    sleep(1); // Simulate some user think time
  });
}

// Function to create user payload
function createUserPayload(userId) {
  return JSON.stringify({
    username: `user${userId}`,
    email: `user${userId}@example.com`,
  });
}

// Function to create reserve payload
function createReservePayload(userId) {
  return JSON.stringify({ user_id: userId });
}

// Function to create grab payload
function createGrabPayload(userId) {
  return JSON.stringify({ user_id: userId });
}
