// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
/* Contract Imports */
/* External Imports */
import { Ownable } from "@openzeppelin/contracts/access/Ownable.sol";
import { IERC20 } from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import { iMVM_DiscountOracle } from "./iMVM_DiscountOracle.sol";
import { Lib_AddressResolver } from "../libraries/resolver/Lib_AddressResolver.sol";
import { MVM_AddressManager } from "../libraries/resolver/MVM_AddressManager.sol";
contract MVM_FeeCollector is Ownable, Lib_AddressResolver {
    event FeeCollected(address collector, address to, uint256 amount)

    // Current l2 gas price
    struct FeeScheme{
       uint8   pctVerifier
    }
    uint256 public discount;
    uint256 public minL2Gas;
    
    mapping (uint256 => FeeScheme) public fee_schemes;
    mapping (uint256 => mapping (address=>bytes)) private challenge_keys;
    mapping (uint256 => mapping (address=>bytes32)) private challenge_key_hashes;
    mapping (uint256 => mapping (address=>bytes)) private challenge_hashes;
    
    address[] public verifiers
    Challenge[] public challenges;
    
    uint public verifyWindow = 3600 * 24; // 24 hours of window to complete the first verify phase
    uint public activeChallenges;

    mapping (address => uint256] public rewards;

    address public metis;

    uint256 public minStake;
    bool allowWithdraw;
    
    constructor(
      address _addressManager,
      address _metis
    )
      Ownable() 
      Lib_AddressResolver(_addressManager)
    {
       minStake = 500 ether;
       metis = _metis;
       allowWithdraw = true;
    }
    
    modifier onlyOperator(uint256 chainID) {
       MVM_AddressManager(resolve("MVM_AddressManager"))
         .getAddress(string(abi.encodePacked(uint2str(chainID), "_Operator"))) == msg.sender;
    }

    function setVerifyWindow (uint256 window) onlyOwner public {
        verifyWindow = window;
    }
    
    // helper fucntion to decrypt the data
    function decrypt(bytes calldata data, bytes calldata key) pure internal returns (bytes) {
      bytes memory decryptedData = data;
      uint j = 0;
      
      for (uint i = 0; i < decryptedData.length; i++, j++) {
          if (j == key.length) {
             j = 0;
          } 
          decryptedData[i] = decryptByte(decryptedData[i], key[j]);
      }

      return decryptedData;
    }

    function decryptByte(bytes1 b, int k) pure internal returns (bytes1) {
      uint16 temp16 = uint16(b);
      if (temp16 > k) {
         temp16 -= k;
      } else {
         temp16 = 256 - k;
      }

      return bytes1(uint8(temp16));
    }
    
    //helper fucntion to encrypt data
    function encrypt(bytes calldata data, bytes calldata key) pure public returns (bytes) {
      bytes memory encryptedData = data;
      uint j = 0;
      
      for (uint i = 0; i < encryptedData.length; i++, j++) {
          if (j == key.length) {
             j = 0;
          } 
          encryptedData[i] = encryptByte(encryptedData[i], key[j]);
      }

      return encryptedData;
    }

    function encryptByte(byte b, int k) pure internal returns (byte4) {
      uint16 temp16 = uint16(b);
      temp16 += k;
      
      if (temp16 > 255) {
         temp16 -= 256;
      } 
      return byte1(uint8(temp16));
    }
    
    // add stake as a verifier
    function verifierStake(uint256 stake) public {
       require(allowWithdraw, "stake is currently prohibited"); //ongoing challenge
       require(stake > 0, "zero stake not allowed");
       require(IERC20(metis).transferFrom(msg.sender, stake), "transfer metis failed");
       if (verifier_stakes[msg.sender] == 0) {
          verifier_stakes[msg.sender] = stake;
          verifiers.push(msg.sender);
       } else {
          verifier_stakes[msg.sender] += stake;
       }
    }
    
    // start a new challenge
    // @param chainID chainid
    // @param index index batch index
    // @param hash  encrypted hash of the correct state
    // @param keyhash hash of the decryption key
    function challenge(uint256 chainID, uint256 index, byte calldata hash, bytes calldata keyhash) public {
       require(verifier_stakes[msg.sender] > minStake, "insufficient stake");
       
       Challenge memory c;
       c.chainID = chainID;
       c.challenger = msg.sender; 
       c.index = index;
       c.timestamp = block.timestamp;
       challenges.push(c);
       uint cIndex = challenges.length;
       
       // house keeping
       challenge_hashes[cIndex][msg.sender] = hash;
       challenge_key_hashes[cIndex][msg.sender] = keyhash;
       challenge[cIndex].numVerify1++;
       
       // prevent stake changes
       allowWithdraw = false;
       activeChallenges++;
       
       emit NewChallenge(cIndex, chainID, index, block.timestamp);
    }

    // phase 1 of the verify, provide an encrypted hash and the hash of the decryption key
    // @param cIndex index of the challenge
    // @param hash encrypted hash of the correct state (for the index referred in the challenge)
    // @param keyhash hash of the decryption key
    function verify1(uint256 cIndex, bytes hash, bytes32 keyhash) public {
       require(verifier_stakes[msg.sender] > minStake, "insufficient stake");
       require(challenge_hashes[cIndex][msg.sender] == [], "verify1 already completed for the sender");
       challenge_hashes[cIndex][msg.sender] = hash;
       challenge_key_hashes[cIndex][msg.sender] = keyhash;
       challenge[cIndex].numVerify1++;
    }
    
    // phase 2 of the verify, provide the actual key to decrypt the hash
    // @param cIndex index of the challenge
    // @param key the decryption key
    function verify2(uint256 cIndex, bytes key) public {
       require(verifier_stakes[msg.sender] > minStake, "insufficient stake");
       require(challenge[cIndex].numVerify1 == verifiers.length 
               || block.timestamp - challenge[cIndex].timestamp > verifyWindow, "phase 2 not ready");
       require(challenge_hashes[cIndex][msg.sender] != [], "you didn't participate in phase 1");   
       if (challenge_keys[cIndex][msg.sender] != []) {
          finalize(cIndex);
          return;
       }
       
       require(sha256(key) == challenge_key_hashes[cIndex][msg.sender], "key and keyhash don't match");
       
       challenge_keys[cIndex][msg.sender] = key;
       challenge_hashes[cIndex][msg.sender] = decrypt(challenge_hashes[cIndex][msg.sender], key);
       challenge[cIndex].numVerify2++;
       finalize(cIndex);
    }
    
    function finalize(uint256 cIndex) internal {
       if (challenge[cIndex].numVerify2 != challenges.length && block.timestamp - challenges[cIndex].timestamp < verifyWindow) {
       }
    }


    function claim() public {
       require(rewards[msg.sender] > 0, "no reward to claim");
       uint256 amount = rewards[msg.sender];
       rewards[msg.sender] = 0;

       require(IERC20(metis).transfer(msg.sender, amount, "token transfer failed"));
    }

    function withdraw(uint256 amount) public {
       require(verifier_stakes[msg.sender] >= amount, "insufficient stake to withdraw");
       uint256 amount = rewards[msg.sender];
       rewards[msg.sender] = 0;

       require(IERC20(metis).transfer(msg.sender, amount, "token transfer failed"));
    }
    
    function setMinStake(
        uint256 _minStake
    )
        public
        onlyOwner
    {
        minStake = _minStake;
    }
    
}
